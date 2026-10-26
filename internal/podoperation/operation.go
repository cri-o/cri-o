/*
Copyright The CRI-O Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package podoperation fences caller-identified checkpoint and restore work.
package podoperation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrExists    = errors.New("pod operation or restore target was already admitted")
	ErrCanceled  = errors.New("pod operation was cancelled")
	ErrActive    = errors.New("pod operation is already active")
	ErrMismatch  = errors.New("pod operation ID was reused with a different request")
	ErrUncertain = errors.New("pod operation has unresolved child work")
)

type Record struct {
	Version          int      `json:"version"`
	ID               string   `json:"id"`
	Kind             int32    `json:"kind"`
	Fingerprint      string   `json:"fingerprint,omitempty"`
	Target           string   `json:"target,omitempty"`
	BootID           string   `json:"bootID"`
	Canceled         bool     `json:"canceled,omitempty"`
	Active           bool     `json:"active,omitempty"`
	Uncertain        bool     `json:"uncertain,omitempty"`
	ChildWork        int      `json:"childWork,omitempty"`
	Settled          bool     `json:"settled,omitempty"`
	Response         []byte   `json:"response,omitempty"`
	Succeeded        bool     `json:"succeeded,omitempty"`
	ResourceID       string   `json:"resourceID,omitempty"`
	OwnerPID         int      `json:"ownerPID,omitempty"`
	OwnerStart       string   `json:"ownerStart,omitempty"`
	MayHaveEffects   bool     `json:"mayHaveEffects,omitempty"`
	Backend          string   `json:"backend,omitempty"`
	BackendResources []string `json:"backendResources,omitempty"`
	BackendSettled   []string `json:"backendSettled,omitempty"`
	TransactionKey   string   `json:"transactionKey,omitempty"`
	FencedResources  []string `json:"fencedResources,omitempty"`
	RemovedResources []string `json:"removedResources,omitempty"`
}

type entry struct {
	mu        sync.Mutex
	record    Record
	cancel    context.CancelFunc
	done      chan struct{}
	cleaning  chan struct{}
	attached  int
	children  map[uint64]context.CancelFunc
	resources map[string]uint64
	childDone map[uint64]chan struct{}
	removing  map[string]chan struct{}
	nextChild uint64
}

const maxJournalRecords = 1024
const maxJournalRecordSize = 1 << 20

// The replay fence is retained even after success. Bound its population rather
// than deleting old identities and allowing delayed requests to execute again.
type Manager struct {
	identities map[string]struct{}
	dir        string
	bootID     string
	ownerStart string
	mu         sync.Mutex
	entries    map[string]*entry
	admission  sync.Mutex
	targets    map[string]string
	// resources maps a kind and resource ID to the operations that recorded
	// it, so sandbox removal does not decode every retained record.
	resources map[string][]string
}

type work struct {
	manager *Manager
	entry   *entry
	cleanup bool
}
type workKey struct{}

func New(dir, bootID string) (*Manager, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("pod operation state path must be absolute")
	}

	if bootID == "" {
		return nil, errors.New("kernel boot ID is required for Pod operation recovery")
	}

	if err := ensureDirectory(dir); err != nil {
		return nil, err
	}

	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New(
			"pod operation state path must be a real directory writable only by its owner",
		)
	}

	start, err := processStart(os.Getpid())
	if err != nil {
		return nil, err
	}

	m := &Manager{
		dir:        dir,
		bootID:     bootID,
		ownerStart: start,
		entries:    make(map[string]*entry),
		targets:    make(map[string]string),
		resources:  make(map[string][]string),
	}

	m.identities = make(map[string]struct{})
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		f, err := os.Open(filepath.Join(dir, file.Name()))
		if err != nil {
			return nil, err
		}

		var identity Record

		decodeErr := json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&identity)

		closeErr := f.Close()
		if err := errors.Join(decodeErr, closeErr); err != nil {
			return nil, err
		}

		record, err := Read(dir, identity.Kind, identity.ID)
		if err != nil {
			return nil, err
		}

		name, err := key(record.Kind, record.ID)
		if err != nil || name != file.Name() {
			return nil, errors.New("pod operation record has inconsistent identity")
		}
		m.identities[name] = struct{}{}

		m.indexResource(record.Kind, record.ResourceID, record.ID)

		if record.Target == "" || record.Settled {
			continue
		}

		if owner, exists := m.targets[record.Target]; exists && owner != name {
			return nil, errors.New("restore target has multiple admitted operations")
		}

		m.targets[record.Target] = name
	}

	return m, nil
}

func ensureDirectory(dir string) error {
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return errors.New("pod operation state path is not a directory")
		}

		return nil
	}

	if !os.IsNotExist(err) {
		return err
	}

	parent := filepath.Dir(dir)
	if parent == dir {
		return err
	}

	if err := ensureDirectory(parent); err != nil {
		return err
	}

	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return err
	}

	f, err := os.Open(parent)
	if err != nil {
		return err
	}
	defer f.Close()

	return f.Sync()
}

func processStart(pid int) (string, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", err
	}

	end := bytes.LastIndexByte(data, ')')
	if end < 0 {
		return "", errors.New("invalid process identity")
	}

	fields := strings.Fields(string(data)[end+1:])
	if len(fields) < 20 {
		return "", errors.New("incomplete process identity")
	}

	return fields[19], nil
}

func ownerGone(r *Record) bool {
	if r.OwnerPID <= 0 || r.OwnerStart == "" {
		return false
	}

	start, err := processStart(r.OwnerPID)

	return os.IsNotExist(err) || (err == nil && start != r.OwnerStart)
}

func recoverable(r *Record, bootID string) bool {
	if r.BootID != bootID {
		return true
	}

	if r.Uncertain {
		return false
	}

	return !r.Active || ownerGone(r)
}

func key(kind int32, id string) (string, error) {
	if (kind != 1 && kind != 2) || id == "" || len(id) > 256 {
		return "", errors.New("valid Pod operation kind and ID are required")
	}

	sum := sha256.Sum256([]byte(id))

	return fmt.Sprintf("%d-%s.json", kind, hex.EncodeToString(sum[:])), nil
}

func (m *Manager) load(kind int32, id string) (*entry, error) {
	name, err := key(kind, id)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()

	e := m.entries[name]
	if e == nil {
		if _, known := m.identities[name]; !known {
			if len(m.identities) >= maxJournalRecords {
				m.mu.Unlock()
				return nil, errors.New("Pod operation journal capacity reached; existing replay fences must be retained")
			}
			m.identities[name] = struct{}{}
		}
		e = &entry{}
		m.entries[name] = e
	}
	m.mu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.record.Version != 0 {
		return e, nil
	}

	r, err := Read(m.dir, kind, id)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if os.IsNotExist(err) {
		r = Record{Version: 1, ID: id, Kind: kind, BootID: m.bootID}
	}

	e.record = r

	return e, nil
}

func Read(dir string, kind int32, id string) (Record, error) {
	name, err := key(kind, id)
	if err != nil {
		return Record{}, err
	}

	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return Record{}, err
	}
	defer f.Close()

	var r Record

	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	if err := decoder.Decode(&r); err != nil {
		return r, err
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return r, errors.New("trailing data in Pod operation record")
	}

	if r.Version != 1 || r.ID != id || r.Kind != kind || r.BootID == "" || r.ChildWork < 0 ||
		(r.ChildWork > 0 && !r.Uncertain) ||
		(r.Active && (r.OwnerPID <= 0 || r.OwnerStart == "")) ||
		(r.Settled && (!r.Canceled || r.Active || r.Uncertain)) {
		return r, errors.New("invalid Pod operation record")
	}

	return r, nil
}

func (m *Manager) save(e *entry) error {
	name, err := key(e.record.Kind, e.record.ID)
	if err != nil {
		return err
	}

	data, err := json.Marshal(&e.record)
	if err != nil {
		return err
	}
	if len(data) > maxJournalRecordSize {
		return errors.New("Pod operation record exceeds size limit")
	}

	f, err := os.CreateTemp(m.dir, ".operation-")
	if err != nil {
		return err
	}

	tmp := f.Name()
	defer os.Remove(tmp)

	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}

	closeErr := f.Close()

	if err != nil {
		return err
	}

	if closeErr != nil {
		return closeErr
	}

	if err := os.Rename(tmp, filepath.Join(m.dir, name)); err != nil {
		return err
	}

	d, err := os.Open(m.dir)
	if err != nil {
		return err
	}
	defer d.Close()

	return d.Sync()
}

// Run records admission before invoking work. Admitted requests are never replayed.
func (m *Manager) Run(
	ctx context.Context,
	kind int32,
	id, fingerprint string,
	fn func(context.Context) ([]byte, error),
) ([]byte, error) {
	return m.RunTarget(ctx, kind, id, fingerprint, "", fn)
}

// RunTarget reserves a restore's complete metadata identity with its durable
// admission. A duplicate must not enter the backend or disturb the first restore.
func (m *Manager) RunTarget(
	ctx context.Context,
	kind int32,
	id, fingerprint, target string,
	fn func(context.Context) ([]byte, error),
) ([]byte, error) {
	if fingerprint == "" {
		return nil, errors.New("pod operation request fingerprint is required")
	}

	m.admission.Lock()
	if _, err := key(kind, id); err != nil {
		m.admission.Unlock()

		return nil, err
	}

	if err := ctx.Err(); err != nil {
		m.admission.Unlock()

		return nil, err
	}

	if target != "" {
		if _, exists := m.targets[target]; exists {
			m.admission.Unlock()

			return nil, ErrExists
		}
	}

	e, err := m.load(kind, id)
	if err != nil {
		m.admission.Unlock()

		return nil, err
	}

	e.mu.Lock()
	if e.record.Canceled || e.record.Fingerprint != "" {
		e.mu.Unlock()
		m.admission.Unlock()

		return nil, ErrExists
	}

	if err := ctx.Err(); err != nil {
		e.mu.Unlock()
		m.admission.Unlock()

		return nil, err
	}

	name, err := key(kind, id)
	if err != nil {
		e.mu.Unlock()
		m.admission.Unlock()

		return nil, err
	}

	e.record.Target = target
	e.record.Fingerprint = fingerprint
	e.record.Active = true
	e.record.BootID = m.bootID
	e.record.OwnerPID = os.Getpid()
	e.record.OwnerStart = m.ownerStart
	opCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	e.cancel = cancel
	stopRequest := context.AfterFunc(ctx, func() {
		e.mu.Lock()
		defer e.mu.Unlock()

		if e.record.Succeeded || !e.record.Active {
			return
		}

		e.record.Canceled = true
		if m.save(e) != nil {
			e.record.Uncertain = true
		}

		cancel()
	})
	e.done = make(chan struct{})

	if target != "" {
		m.targets[target] = name
	}

	if err = m.save(e); err != nil {
		stopRequest()
		cancel()

		e.record.Active = false
		e.cancel = nil
		e.done = nil
		e.mu.Unlock()
		m.admission.Unlock()

		return nil, err
	}
	e.mu.Unlock()
	m.admission.Unlock()

	retainOwner := false
	defer func() {
		if !retainOwner {
			cancel()
		}
	}()

	completed := false
	defer func() {
		if completed {
			return
		}

		e.mu.Lock()
		defer e.mu.Unlock()

		e.record.Uncertain = true
		e.record.Active = false

		stopRequest()

		_ = m.save(e) //nolint:errcheck // memory keeps the uncertainty if this save fails
		e.cancel = nil
		close(e.done)
		e.done = nil
	}()

	response, runErr := fn(context.WithValue(opCtx, workKey{}, &work{manager: m, entry: e}))
	e.mu.Lock()

	e.record.Active = false
	if e.record.Canceled || ctx.Err() != nil || opCtx.Err() != nil {
		runErr = errors.Join(runErr, ctx.Err(), context.Cause(opCtx), ErrCanceled)
	}

	if e.record.Uncertain || e.record.ChildWork != 0 {
		runErr = errors.Join(runErr, ErrUncertain)
	}

	if runErr == nil {
		e.record.Response = append([]byte(nil), response...)
		e.record.Succeeded = true
	}

	persistErr := m.save(e)
	if persistErr != nil {
		e.record.Uncertain = true
	}

	stopRequest()

	retainOwner = runErr == nil && persistErr == nil && kind == 2
	if !retainOwner {
		e.cancel = nil
	}

	close(e.done)
	e.done = nil
	completed = true
	e.mu.Unlock()

	return response, errors.Join(runErr, persistErr)
}

// Cancel writes its fence before waiting. Cleanup owns the operation after its
// original worker has returned; a second caller waits for that cleanup owner.
func (m *Manager) Cancel(
	ctx context.Context,
	kind int32,
	id string,
	cleanup func(context.Context, *Record) error,
) error {
	return m.Settle(ctx, kind, id, nil, cleanup)
}

// Settle can reconcile uncertain backend work only through an authoritative
// abort-and-wait callback. A transport error or an empty resource cache is not
// such an acknowledgement.
func (m *Manager) Settle(
	ctx context.Context,
	kind int32,
	id string,
	backend func(context.Context, *Record) error,
	cleanup func(context.Context, *Record) error,
) error {
	e, err := m.load(kind, id)
	if err != nil {
		return err
	}

	for {
		e.mu.Lock()

		e.record.Canceled = true
		if err = m.save(e); err != nil {
			e.mu.Unlock()

			return err
		}

		if e.record.Settled {
			e.mu.Unlock()
			m.releaseTarget(e)

			return nil
		}

		if e.cancel != nil {
			e.cancel()
		}

		for _, cancel := range e.children {
			cancel()
		}

		wait := e.done
		if wait == nil {
			wait = e.cleaning
		}

		if wait != nil {
			e.mu.Unlock()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-wait:
				continue
			}
		}

		if !recoverable(&e.record, m.bootID) {
			if backend == nil {
				e.mu.Unlock()

				return ErrUncertain
			}

			e.cleaning = make(chan struct{})
			record := e.record
			e.mu.Unlock()

			backendErr := backend(ctx, &record)

			e.mu.Lock()
			if backendErr == nil {
				e.record.Active = false
				e.record.Uncertain = false
				e.record.ChildWork = 0

				backendErr = m.save(e)
				if backendErr != nil {
					e.record.Uncertain = true
				}
			}

			close(e.cleaning)
			e.cleaning = nil
			e.mu.Unlock()

			if backendErr != nil {
				return errors.Join(ErrUncertain, backendErr)
			}

			continue
		}
		// Another boot proves old child processes are gone, but resource cleanup
		// still has to complete before this record can be acknowledged.
		e.record.Active = false
		e.record.Uncertain = false
		e.record.ChildWork = 0
		e.record.BootID = m.bootID
		e.record.OwnerPID = os.Getpid()
		e.record.OwnerStart = m.ownerStart

		e.record.Active = true
		if err = m.save(e); err != nil {
			e.mu.Unlock()

			return err
		}

		e.cleaning = make(chan struct{})
		record := e.record
		e.mu.Unlock()
		cleanupErr := m.cleanup(ctx, e, &record, cleanup)
		e.mu.Lock()

		e.record.Active = false
		if cleanupErr == nil && !e.record.Uncertain {
			e.record.Settled = true
		}

		persistErr := m.save(e)
		close(e.cleaning)
		e.cleaning = nil
		uncertain := e.record.Uncertain
		settled := persistErr == nil && e.record.Settled
		e.mu.Unlock()

		if settled {
			m.releaseTarget(e)
		}

		if uncertain {
			cleanupErr = errors.Join(cleanupErr, ErrUncertain)
		}

		return errors.Join(cleanupErr, persistErr)
	}
}

func (m *Manager) releaseTarget(e *entry) {
	m.admission.Lock()
	defer m.admission.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.record.Settled {
		return
	}

	name, err := key(e.record.Kind, e.record.ID)
	if err == nil && m.targets[e.record.Target] == name {
		delete(m.targets, e.record.Target)
	}

	m.forgetSettled(e)
}

// Prepared distinguishes normal successful RPC teardown from interrupted work.
func (m *Manager) Prepared(kind int32, id string) bool {
	e, err := m.load(kind, id)
	if err != nil {
		return false
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	return e.record.Succeeded && !e.record.Canceled
}

// Settled fences remain on disk; their cache entries do not retain memory for
// every Pod UID ever admitted by a long-lived daemon.
func (m *Manager) forgetSettled(e *entry) {
	name, err := key(e.record.Kind, e.record.ID)
	if err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.entries[name] == e {
		delete(m.entries, name)
	}
}

func (m *Manager) cleanup(
	ctx context.Context,
	e *entry,
	record *Record,
	fn func(context.Context, *Record) error,
) error {
	completed := false
	defer func() {
		if completed {
			return
		}

		e.mu.Lock()
		defer e.mu.Unlock()

		e.record.Uncertain = true
		e.record.Active = false
		_ = m.save(e) //nolint:errcheck // memory keeps the uncertainty if this save fails
		close(e.cleaning)
		e.cleaning = nil
	}()

	err := fn(context.WithValue(ctx, workKey{}, &work{manager: m, entry: e, cleanup: true}), record)
	completed = true

	return err
}

// Join keeps an admitted lower-layer request attached to this owner after its
// caller stops waiting. A failed lower-layer call retains uncertainty because
// the remote worker may outlive the RPC connection.
func Join(ctx context.Context, fn func(context.Context) error) error {
	w := workFrom(ctx)
	if w == nil {
		return fn(ctx)
	}

	w.entry.mu.Lock()
	if !w.cleanup && (w.entry.record.Canceled || ctx.Err() != nil) {
		w.entry.mu.Unlock()

		return ErrCanceled
	}

	w.entry.record.ChildWork++
	w.entry.record.MayHaveEffects = true
	w.entry.record.Uncertain = true
	err := w.manager.save(w.entry)
	w.entry.mu.Unlock()

	if err != nil {
		return err
	}

	if err := fn(ctx); err != nil {
		return err
	}

	w.entry.mu.Lock()
	w.entry.record.ChildWork--
	w.entry.record.Uncertain = w.entry.record.ChildWork > 0
	err = w.manager.save(w.entry)
	w.entry.mu.Unlock()

	return err
}

// JoinSynchronous is Join for a lower-layer call that has no detached worker.
// When the call returns without cancellation, its error is authoritative, so
// the operation can still recover the effects of earlier completed calls.
func JoinSynchronous(ctx context.Context, fn func(context.Context) error) error {
	w := workFrom(ctx)
	if w == nil {
		return fn(ctx)
	}

	return Join(ctx, func(joined context.Context) error {
		err := fn(joined)
		if err == nil || joined.Err() != nil {
			return err
		}

		w.entry.mu.Lock()
		defer w.entry.mu.Unlock()

		if w.entry.record.Canceled {
			return err
		}

		w.entry.record.ChildWork--

		w.entry.record.Uncertain = w.entry.record.ChildWork > 0
		if saveErr := w.manager.save(w.entry); saveErr != nil {
			w.entry.record.Uncertain = true

			return errors.Join(err, saveErr)
		}

		return err
	})
}

func workFrom(ctx context.Context) *work {
	w, ok := ctx.Value(workKey{}).(*work)
	if !ok {
		return nil
	}

	return w
}

func ID(ctx context.Context) string {
	w := workFrom(ctx)
	if w == nil {
		return ""
	}

	return w.entry.record.ID
}

func IsCleanup(ctx context.Context) bool {
	w := workFrom(ctx)

	return w != nil && w.cleanup
}

func IsCleanupFor(ctx context.Context, kind int32, id string) bool {
	w := workFrom(ctx)

	return w != nil && w.cleanup && w.entry.record.ID == id && w.entry.record.Kind == kind
}

func SetTransaction(ctx context.Context, key string) error {
	w := workFrom(ctx)
	if w == nil {
		return nil
	}

	w.entry.mu.Lock()
	defer w.entry.mu.Unlock()

	if key == "" || (w.entry.record.TransactionKey != "" && w.entry.record.TransactionKey != key) {
		return ErrMismatch
	}

	w.entry.record.TransactionKey = key

	return w.manager.save(w.entry)
}

func SetResource(ctx context.Context, id string, backend ...string) error {
	if id == "" {
		return errors.New("pod operation resource identity must not be empty")
	}

	w := workFrom(ctx)
	if w == nil {
		return nil
	}

	w.entry.mu.Lock()
	defer w.entry.mu.Unlock()

	if w.entry.record.ResourceID != "" && w.entry.record.ResourceID != id {
		return ErrMismatch
	}

	w.entry.record.ResourceID = id
	w.manager.mu.Lock()
	w.manager.indexResource(w.entry.record.Kind, id, w.entry.record.ID)
	w.manager.mu.Unlock()

	if len(backend) > 0 {
		if w.entry.record.Backend != "" && w.entry.record.Backend != backend[0] {
			return ErrMismatch
		}

		w.entry.record.Backend = backend[0]
	}

	w.entry.record.MayHaveEffects = true

	return w.manager.save(w.entry)
}

// RegisterBackend records a stable backend container identity before launch.
func RegisterBackend(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("backend container identity is required")
	}

	w := workFrom(ctx)
	if w == nil {
		return nil
	}

	w.entry.mu.Lock()
	defer w.entry.mu.Unlock()

	if w.entry.record.Canceled || ctx.Err() != nil {
		return ErrCanceled
	}

	if slices.Contains(w.entry.record.BackendResources, id) {
		return nil
	}

	w.entry.record.BackendResources = append(w.entry.record.BackendResources, id)

	return w.manager.save(w.entry)
}

func CheckJoined(ctx context.Context) error {
	w := workFrom(ctx)
	if w == nil {
		return nil
	}

	w.entry.mu.Lock()
	defer w.entry.mu.Unlock()

	if w.entry.record.Uncertain {
		return ErrUncertain
	}

	return nil
}

func Directory(root, scope string) string {
	sum := sha256.Sum256([]byte(scope))

	return filepath.Join(root, hex.EncodeToString(sum[:]))
}

func CanRecover(ctx context.Context, dir, bootID string, kind int32, id string) error {
	if id == "" {
		return nil
	}

	if w := workFrom(ctx); w != nil && w.manager.dir == dir &&
		w.entry.record.ID == id &&
		w.entry.record.Kind == kind {
		w.entry.mu.Lock()
		defer w.entry.mu.Unlock()

		if w.entry.record.Uncertain {
			return ErrUncertain
		}

		return nil
	}

	r, err := Read(dir, kind, id)
	if err != nil {
		return err
	}

	if !recoverable(&r, bootID) {
		return ErrUncertain
	}

	return nil
}

// WithCleanup carries ownership into cleanup that must finish after cancellation.
func WithCleanup(ctx, owner context.Context) context.Context {
	if w, ok := owner.Value(workKey{}).(*work); ok {
		return context.WithValue(
			ctx,
			workKey{},
			&work{manager: w.manager, entry: w.entry, cleanup: true},
		)
	}

	return ctx
}

// Attach admits work on resources returned by a completed operation. Restore
// starts use the same fence as preparation, so removal cannot overtake a start
// that already obtained its container before cancellation.
func (m *Manager) Attach(
	ctx context.Context,
	kind int32,
	id string,
	fn func(context.Context) error,
) (retErr error) {
	return m.AttachResource(ctx, kind, id, "", fn)
}

func (m *Manager) AttachResource(
	ctx context.Context,
	kind int32,
	id, resource string,
	fn func(context.Context) error,
) (retErr error) {
	e, err := m.load(kind, id)
	if err != nil {
		return err
	}

	e.mu.Lock()
	if resource != "" &&
		(slices.Contains(e.record.FencedResources, resource) || e.resources[resource] != 0) {
		e.mu.Unlock()

		return ErrExists
	}

	if e.record.Canceled {
		e.mu.Unlock()

		return ErrCanceled
	}

	if err := ctx.Err(); err != nil {
		e.mu.Unlock()

		return err
	}

	if !e.record.Succeeded || e.record.Uncertain || (e.record.Active && e.attached == 0) {
		// Attached children intentionally retain uncertainty until all have
		// joined, but a failed child leaves more child work than attachments.
		if e.attached == 0 || e.record.ChildWork > e.attached {
			e.mu.Unlock()

			return ErrUncertain
		}
	}

	if e.record.Fingerprint == "" || !e.record.Succeeded {
		e.mu.Unlock()

		return ErrUncertain
	}

	if e.attached == 0 {
		e.done = make(chan struct{})
	}

	e.attached++

	opCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()

	if e.children == nil {
		e.children = make(map[uint64]context.CancelFunc)
		e.resources = make(map[string]uint64)
		e.childDone = make(map[uint64]chan struct{})
	}

	e.nextChild++
	childID := e.nextChild
	e.children[childID] = cancel

	e.childDone[childID] = make(chan struct{})
	if resource != "" {
		e.resources[resource] = childID
		e.record.FencedResources = append(e.record.FencedResources, resource)
	}

	stopRequest := context.AfterFunc(ctx, func() {
		e.mu.Lock()
		defer e.mu.Unlock()

		if _, active := e.children[childID]; !active {
			return
		}

		e.record.Canceled = true
		if m.save(e) != nil {
			e.record.Uncertain = true
		}

		cancel()
	})
	e.record.Active = true
	e.record.OwnerPID = os.Getpid()
	e.record.OwnerStart = m.ownerStart
	e.record.BootID = m.bootID
	e.record.ChildWork++
	e.record.Uncertain = true
	e.record.MayHaveEffects = true

	err = m.save(e)
	if err != nil {
		e.attached--

		stopRequest()
		delete(e.children, childID)
		delete(e.resources, resource)
		close(e.childDone[childID])
		delete(e.childDone, childID)

		if e.attached == 0 {
			close(e.done)
			e.done = nil
			e.record.Active = false
		}
		e.mu.Unlock()

		return err
	}
	e.mu.Unlock()

	completed := false

	defer func() {
		e.mu.Lock()
		defer e.mu.Unlock()

		if completed && retErr == nil && opCtx.Err() == nil && ctx.Err() == nil {
			e.record.ChildWork--
		} else if opCtx.Err() != nil || ctx.Err() != nil {
			retErr = errors.Join(retErr, opCtx.Err(), ctx.Err())
		}

		stopRequest()
		delete(e.children, childID)
		delete(e.resources, resource)
		close(e.childDone[childID])
		delete(e.childDone, childID)
		e.record.Uncertain = e.record.ChildWork > 0 || !completed

		e.attached--
		if e.record.ChildWork > e.attached {
			retErr = errors.Join(retErr, ErrUncertain)
		}

		if e.attached == 0 {
			e.record.Active = false
		}

		persistErr := m.save(e)
		if persistErr != nil {
			e.record.Uncertain = true
		}

		if e.attached == 0 {
			close(e.done)
			e.done = nil
		}

		retErr = errors.Join(retErr, persistErr)
	}()

	retErr = fn(context.WithValue(opCtx, workKey{}, &work{manager: m, entry: e}))
	completed = true

	return retErr
}

// OperationForResource also works after removal has erased the resource cache
// and receipt, so a failed final journal sync can be retried after restart.
func (m *Manager) OperationForResource(kind int32, resource string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// A caller may already hold a removed container while another owner
	// reuses its name. Keep routing that stale resource to its old fence.
	owners := m.resources[resourceKey(kind, resource)]
	if len(owners) > 1 {
		return "", errors.New("resource has multiple Pod operation owners")
	}

	if len(owners) == 0 {
		return "", nil
	}

	return owners[0], nil
}

func resourceKey(kind int32, resource string) string {
	return strconv.Itoa(int(kind)) + "/" + resource
}

func (m *Manager) indexResource(kind int32, resource, id string) {
	if resource == "" {
		return
	}

	key := resourceKey(kind, resource)
	if !slices.Contains(m.resources[key], id) {
		m.resources[key] = append(m.resources[key], id)
	}
}

// RemoveResource fences only this container. Normal garbage collection must
// not cancel a healthy sibling or release the whole sandbox's metadata.
func (m *Manager) RemoveResource(
	ctx context.Context,
	kind int32,
	id, resource string,
	backend func(context.Context, *Record) error,
	remove func(context.Context) error,
) (retErr error) {
	e, err := m.load(kind, id)
	if err != nil {
		return err
	}

	for {
		e.mu.Lock()
		if e.record.Settled {
			err := m.save(e)
			e.mu.Unlock()

			if err == nil {
				m.releaseTarget(e)
			}

			return err
		}

		if slices.Contains(e.record.RemovedResources, resource) {
			err := m.save(e)
			e.mu.Unlock()

			return err
		}

		if e.removing == nil {
			e.removing = make(map[string]chan struct{})
		}

		wait := e.removing[resource]
		if wait == nil {
			wait = e.cleaning
		}

		if child := e.resources[resource]; wait == nil && child != 0 {
			wait = e.childDone[child]
		}

		if wait == nil && e.attached == 0 {
			// RunTarget owns done without attaching. Removal must not replace
			// that channel while the original worker can still close it.
			wait = e.done
		}

		if !slices.Contains(e.record.FencedResources, resource) {
			e.record.FencedResources = append(e.record.FencedResources, resource)
		}

		if err := m.save(e); err != nil {
			e.mu.Unlock()

			return err
		}

		if wait != nil {
			e.mu.Unlock()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-wait:
				continue
			}
		}

		if err := ctx.Err(); err != nil {
			e.mu.Unlock()

			return err
		}

		if e.attached == 0 {
			e.done = make(chan struct{})
		}

		e.attached++
		e.record.Active = true
		e.record.BootID = m.bootID
		e.record.OwnerPID = os.Getpid()
		e.record.OwnerStart = m.ownerStart
		e.removing[resource] = make(chan struct{})

		record := e.record
		if err := m.save(e); err != nil {
			e.attached--
			close(e.removing[resource])
			delete(e.removing, resource)

			if e.attached == 0 {
				e.record.Active = false
				close(e.done)
				e.done = nil
			}
			e.mu.Unlock()

			return err
		}
		e.mu.Unlock()
		owner := context.WithValue(ctx, workKey{}, &work{manager: m, entry: e, cleanup: true})
		completed := false

		//nolint:gocritic // the loop returns after this iteration registers the defer
		defer func() {
			e.mu.Lock()
			defer e.mu.Unlock()

			if completed && retErr == nil {
				e.record.RemovedResources = append(e.record.RemovedResources, resource)
			} else if !completed {
				e.record.Uncertain = true
			}

			e.attached--
			if e.attached == 0 {
				e.record.Active = false
			}

			retErr = errors.Join(retErr, m.save(e))
			close(e.removing[resource])
			delete(e.removing, resource)

			if e.attached == 0 {
				close(e.done)
				e.done = nil
			}
		}()

		if slices.Contains(record.BackendResources, resource) &&
			!slices.Contains(record.BackendSettled, resource) {
			if backend == nil {
				retErr = ErrUncertain
			} else {
				retErr = backend(owner, &record)
			}

			if retErr == nil {
				e.mu.Lock()
				e.record.BackendSettled = append(e.record.BackendSettled, resource)
				retErr = m.save(e)
				e.mu.Unlock()
			}
		}

		if retErr == nil {
			retErr = Join(owner, remove)
		}

		completed = true

		return retErr
	}
}
