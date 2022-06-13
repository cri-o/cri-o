package verify

import (
	"github.com/theupdateframework/go-tuf/data"
	"github.com/theupdateframework/go-tuf/internal/roles"
	"github.com/theupdateframework/go-tuf/pkg/keys"
)

type Role struct {
	KeyIDs    map[string]struct{}
	Threshold int
}

func (r *Role) ValidKey(id string) bool {
	_, ok := r.KeyIDs[id]
	return ok
}

type DB struct {
	roles     map[string]*Role
	verifiers map[string]keys.Verifier
}

func NewDB() *DB {
	return &DB{
		roles:     make(map[string]*Role),
		verifiers: make(map[string]keys.Verifier),
	}
}

// NewDBFromDelegations returns a DB that verifies delegations
// of a given Targets.
func NewDBFromDelegations(d *data.Delegations) (*DB, error) {
	db := &DB{
		roles:     make(map[string]*Role, len(d.Roles)),
		verifiers: make(map[string]keys.Verifier, len(d.Keys)),
	}
	for _, r := range d.Roles {
		if _, ok := roles.TopLevelRoles[r.Name]; ok {
			return nil, ErrInvalidDelegatedRole
		}
		role := &data.Role{Threshold: r.Threshold, KeyIDs: r.KeyIDs}
		if err := db.AddRole(r.Name, role); err != nil {
			return nil, err
		}
	}
	for id, k := range d.Keys {
		if err := db.AddKey(id, k); err != nil {
			return nil, err
		}
	}
	return db, nil
}

func (db *DB) AddKey(id string, k *data.PublicKey) error {
	if !k.ContainsID(id) {
		return ErrWrongID{}
	}
	verifier, err := keys.GetVerifier(k)
	if err != nil {
		return ErrInvalidKey
	}
	db.verifiers[id] = verifier
	return nil
}

func (db *DB) AddRole(name string, r *data.Role) error {
	if r.Threshold < 1 {
		return ErrInvalidThreshold
	}

	role := &Role{
		KeyIDs:    make(map[string]struct{}),
		Threshold: r.Threshold,
	}
	for _, id := range r.KeyIDs {
		if len(id) != data.KeyIDLength {
			return ErrInvalidKeyID
		}
		role.KeyIDs[id] = struct{}{}
	}

	db.roles[name] = role
	return nil
}

func (db *DB) GetVerifier(id string) (keys.Verifier, error) {
	k, ok := db.verifiers[id]
	if !ok {
		return nil, ErrMissingKey
	}
	return k, nil
}

func (db *DB) GetRole(name string) *Role {
	return db.roles[name]
}
