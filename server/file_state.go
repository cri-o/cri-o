package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containers/storage/storage"
	"github.com/kubernetes-incubator/cri-o/oci"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// The state is a directory on disk containing a lockfile (STATE_LOCK) and a number of sandboxes (directories)
// Each sandbox has a description file (description.json) containing basic information about it
// Containers are individual files within a sandbox's folder named by their full idea and '.json'
// JSON encoding is was used during development mainly for ease of debugging.
// For now, there is a single global lockfile. Eventually, it is desired to move toward a global lock for
// sandbox creation/deletion and additional, separate per-sandbox locks to improve performance.

// FileState is a file-based store for CRI-O's state
// It allows multiple programs (e.g. kpod and CRI-O) to interact with the same set of containers without races
type FileState struct {
	rootPath    string
	lockfile    storage.Locker
	memoryState StateStore
}

// Net namespace is taken from enclosing sandbox
// State is not included at all, we assume the runtime has it
type containerFile struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	BundlePath  string                `json:"bundlePath"`
	LogPath     string                `json:"logPath"`
	Labels      fields.Set            `json:"labels"`
	Annotations fields.Set            `json:"annotations"`
	Image       *pb.ImageSpec         `json:"image"`
	Sandbox     string                `json:"sandbox"`
	Terminal    bool                  `json:"terminal"`
	Privileged  bool                  `json:"privileged"`
	Metadata    *pb.ContainerMetadata `json:"metadata"`
}

type sandboxFile struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	LogDir           string                 `json:"logDir"`
	Labels           fields.Set             `json:"labels"`
	Annotations      map[string]string      `json:"annotations"`
	InfraContainer   string                 `json:"infraContainer"` // ID of infra container
	Containers       []string               `json:"containers"`     // List of IDs
	ProcessLabel     string                 `json:"processLabel"`
	MountLabel       string                 `json:"mountLabel"`
	NetNsPath        string                 `json:"netNsPath"`
	NetNsSymlinkPath string                 `json:"netNsSymlinkPath"`
	NetNsClosed      bool                   `json:"netNsClosed"`
	NetNsRestored    bool                   `json:"netNsRestored"`
	Metadata         *pb.PodSandboxMetadata `json:"metadata"`
	ShmPath          string                 `json:"shmPath"`
	CgroupParent     string                 `json:"cgroupParent"`
	Privileged       bool                   `json:"privileged"`
}

// Sync the in-memory state and the state on disk
// Conditional on state being modified
// For now, this uses a brute-force approach - when on-disk state is modified,
// throw away the in-memory state and rebuild it entirely from on-disk state
func (s *FileState) syncWithDisk() error {
	modified, err := s.lockfile.Modified()
	if err != nil {
		return fmt.Errorf("file locking error: %v", err)
	} else if !modified {
		// On-disk state unmodified, don't need to do anything
		return nil
	}

	// Get a list of all directories under the root path - each should be a sandbox
	dirListing, err := ioutil.ReadDir(s.rootPath)
	if err != nil {
		return fmt.Errorf("error listing contents of root path: %v", err)
	}

	newState := NewInMemoryState()

	// Loop through contents of the root directory, transforming all directories into sandboxes
	for _, file := range dirListing {
		if !file.IsDir() {
			continue
		}

		// The folder's name should be the sandbox ID
		sandbox, err := s.getSandboxFromDisk(file.Name())
		if err != nil {
			return err
		}

		if err := newState.AddSandbox(sandbox); err != nil {
			return fmt.Errorf("error populating new state: %v", err)
		}
	}

	s.memoryState = newState

	return nil
}

// Convert a sandbox to on-disk format
func sandboxToSandboxFile(sb *sandbox) *sandboxFile {
	sbFile := sandboxFile{
		ID:               sb.id,
		Name:             sb.name,
		LogDir:           sb.logDir,
		Labels:           sb.labels,
		Annotations:      sb.annotations,
		Containers:       make([]string, 0, len(sb.containers.List())),
		ProcessLabel:     sb.processLabel,
		MountLabel:       sb.mountLabel,
		Metadata:         sb.metadata,
		ShmPath:          sb.shmPath,
		CgroupParent:     sb.cgroupParent,
		Privileged:       sb.privileged,
	}

	if sb.netns != nil {
		sbFile.NetNsPath = sb.netNsPath()
		sbFile.NetNsClosed = sb.netns.closed
		sbFile.NetNsRestored = sb.netns.restored

		if sb.netns.symlink != nil {
			sbFile.NetNsSymlinkPath = sb.netns.symlink.Name()
		}
	}

	for _, ctr := range sb.containers.List() {
		sbFile.Containers = append(sbFile.Containers, ctr.ID())
	}

	sbFile.InfraContainer = sb.infraContainer.ID()

	return &sbFile
}

// Convert a sandbox from on-disk format to normal format
func (s *FileState) sandboxFileToSandbox(sbFile *sandboxFile) (*sandbox, error) {
	sb := sandbox{
		id:           sbFile.ID,
		name:         sbFile.Name,
		logDir:       sbFile.LogDir,
		labels:       sbFile.Labels,
		annotations:  sbFile.Annotations,
		containers:   oci.NewMemoryStore(),
		processLabel: sbFile.ProcessLabel,
		mountLabel:   sbFile.MountLabel,
		metadata:     sbFile.Metadata,
		shmPath:      sbFile.ShmPath,
		cgroupParent: sbFile.CgroupParent,
		privileged:   sbFile.Privileged,
	}

	ns, err := ns.GetNS(sbFile.NetNsPath)
	if err != nil {
		return nil, fmt.Errorf("error retrieving network namespace %v: %v", sbFile.NetNsPath, err)
	}
	var symlink *os.File
	if sbFile.NetNsSymlinkPath != "" {
		symlink, err = os.Open(sbFile.NetNsSymlinkPath)
		if err != nil {
			return nil, fmt.Errorf("error retrieving network namespace symlink %v: %v", sbFile.NetNsSymlinkPath, err)
		}
	}
	netns := sandboxNetNs{
		ns:       ns,
		symlink:  symlink,
		closed:   sbFile.NetNsClosed,
		restored: sbFile.NetNsRestored,
	}
	sb.netns = &netns

	infraCtr, err := s.getContainerFromDisk(sbFile.InfraContainer, sbFile.ID, &netns)
	if err != nil {
		return nil, fmt.Errorf("error retrieving infra container for pod %v: %v", sbFile.ID, err)
	}
	sb.infraContainer = infraCtr

	for _, id := range sbFile.Containers {
		ctr, err := s.getContainerFromDisk(id, sbFile.ID, &netns)
		if err != nil {
			return nil, fmt.Errorf("error retrieving container ID %v in pod ID %v: %v", id, sbFile.ID, err)
		}
		sb.containers.Add(ctr.ID(), ctr)
	}

	return &sb, nil
}

// Retrieve a sandbox and all associated containers from disk
func (s *FileState) getSandboxFromDisk(id string) (*sandbox, error) {
	sbFile, err := s.getSandboxFileFromDisk(id)
	if err != nil {
		return nil, err
	}

	return s.sandboxFileToSandbox(sbFile)
}

// Retrieve a sandbox file from disk
func (s *FileState) getSandboxFileFromDisk(id string) (*sandboxFile, error) {
	sbExists, err := s.checkSandboxExistsOnDisk(id)
	if err != nil {
		return nil, err
	} else if !sbExists {
		return nil, fmt.Errorf("sandbox with ID %v does not exist on disk", id)
	}

	_, descriptionFilePath := s.getSandboxPath(id)
	sbFile := sandboxFile{}

	if err = decodeFromFile(descriptionFilePath, &sbFile); err != nil {
		return nil, fmt.Errorf("error retrieving sandbox %v from disk: %v", id, err)
	}

	return &sbFile, err
}

// Save a sandbox to disk
// Will save all associated containers, including infra container, as well
func (s *FileState) putSandboxToDisk(sb *sandbox) error {
	sbFile := sandboxToSandboxFile(sb)

	if err := s.putSandboxFileToDisk(sbFile); err != nil {
		return err
	}

	// Need to put infra container and any additional containers to disk as well
	if err := s.putContainerToDisk(sb.infraContainer, false); err != nil {
		return fmt.Errorf("error storing sandbox %v infra container: %v", sb.id, err)
	}

	for _, ctr := range sb.containers.List() {
		if err := s.putContainerToDisk(ctr, false); err != nil {
			return fmt.Errorf("error storing container %v in sandbox %v: %v", ctr.ID(), sb.id, err)
		}
	}

	return nil
}

// Save a sandbox file to disk
// If sandbox already exists on disk, will cowardly refuse to replace it
func (s *FileState) putSandboxFileToDisk(sbFile *sandboxFile) error {
	sbExists, err := s.checkSandboxExistsOnDisk(sbFile.ID)
	if err != nil {
		return err
	} else if sbExists {
		return fmt.Errorf("sandbox with ID %v already exists on disk, cowardly refusing to replace", sbFile.ID)
	}

	folderPath, filePath := s.getSandboxPath(sbFile.ID)

	// Make the folder first
	if err := os.Mkdir(folderPath, 0700); err != nil {
		return fmt.Errorf("error creating folder for sandbox ID %v: %v", sbFile.ID, err)
	}

	// Then encode the sandbox description data to disk
	if err := encodeToFile(filePath, sbFile); err != nil {
		if err2 := os.RemoveAll(folderPath); err2 != nil {
			logrus.Errorf("error removing incomplete sandbox %v: %v", sbFile.ID, err2)
		}

		return fmt.Errorf("error encoding sandbox ID %v description data to disk: %v", sbFile.ID, err)
	}

	if err := s.lockfile.Touch(); err != nil {
		logrus.Errorf("error updating lockfile writer: %v", err)
	}

	return nil
}

// Update a sandbox's description file on disk (e.g. to add/remove a container from state)
func (s *FileState) updateSandboxFileOnDisk(sbFileNew *sandboxFile) error {
	sbExists, err := s.checkSandboxExistsOnDisk(sbFileNew.ID)
	if err != nil {
		return err
	} else if !sbExists {
		return fmt.Errorf("cannot update sandbox ID %v as it does not exist on disk", sbFileNew.ID)
	}

	// Delete the existing sandbox description file first
	_, sbFilePath := s.getSandboxPath(sbFileNew.ID)
	if err := os.Remove(sbFilePath); err != nil {
		return fmt.Errorf("error removing sandbox file to update sandbox %v: %v", sbFileNew.ID, err)
	}

	if err := encodeToFile(sbFilePath, sbFileNew); err != nil {
		return err
	}

	if err := s.lockfile.Touch(); err != nil {
		logrus.Errorf("error updating lockfile writer: %v", err)
	}

	return nil
}

// Remove a sandbox from disk
// TODO: maybe remove the description file first, to ensure we don't have a potentially valid sandbox at the end?
func (s *FileState) removeSandboxFromDisk(id string) error {
	sbExists, err := s.checkSandboxExistsOnDisk(id)
	if err != nil {
		return err
	} else if !sbExists {
		return fmt.Errorf("cannot remove sandbox ID %v as it does not exist on disk", id)
	}

	sbDir, _ := s.getSandboxPath(id)

	if err := s.lockfile.Touch(); err != nil {
		logrus.Errorf("error updating lockfile writer: %v", err)
	}

	if err := os.RemoveAll(sbDir); err != nil {
		return fmt.Errorf("error removing sandbox ID %v: %v", id, err)
	}

	return nil
}

// Check if a sandbox exists on disk and is sanely formatted
// Does not validate sandbox description data
func (s *FileState) checkSandboxExistsOnDisk(id string) (bool, error) {
	folderPath, filePath := s.getSandboxPath(id)

	folderStat, err := os.Stat(folderPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("error accessing sandbox folder %v: %v", folderPath, err)
	}

	if !folderStat.IsDir() {
		return false, fmt.Errorf("sandbox folder %v is not a folder", folderPath)
	}

	// Don't need to IsNotExist check here - a sandbox folder without a description file is unusable
	// So any error is bad
	if _, err := os.Stat(filePath); err != nil {
		return false, fmt.Errorf("sandbox folder %v exists but description file %v cannot be accessed: %v", folderPath, filePath, err)
	}

	return true, nil
}

// Get path of a sandbox on disk
// Returns two strings: the first is the path of the sandbox's folder, the second the sandbox's JSON description file
func (s *FileState) getSandboxPath(id string) (string, string) {
	folderPath := path.Join(s.rootPath, id)
	filePath := path.Join(folderPath, "description.json")

	return folderPath, filePath
}

// Convert oci.Container to on-disk container format
func getContainerFileFromContainer(ctr *oci.Container) *containerFile {
	ctrFile := containerFile{
		ID:          ctr.ID(),
		Name:        ctr.Name(),
		BundlePath:  ctr.BundlePath(),
		LogPath:     ctr.LogPath(),
		Labels:      ctr.Labels(),
		Annotations: ctr.Annotations(),
		Image:       ctr.Image(),
		Sandbox:     ctr.Sandbox(),
		Terminal:    ctr.Terminal(),
		Privileged:  ctr.Privileged(),
		Metadata:    ctr.Metadata(),
	}

	return &ctrFile
}

// Convert on-disk container format to normal oci.Container
func getContainerFromContainerFile(ctrFile *containerFile, netNs *sandboxNetNs) (*oci.Container, error) {
	return oci.NewContainer(ctrFile.ID, ctrFile.Name, ctrFile.BundlePath, ctrFile.LogPath, netNs.ns, ctrFile.Labels, ctrFile.Annotations, ctrFile.Image, ctrFile.Metadata, ctrFile.Sandbox, ctrFile.Terminal, ctrFile.Privileged)
}

// Get a container from disk
func (s *FileState) getContainerFromDisk(id, sandboxID string, netNs *sandboxNetNs) (*oci.Container, error) {
	ctrFile, err := s.getContainerFileFromDisk(id, sandboxID)
	if err != nil {
		return nil, err
	}

	return getContainerFromContainerFile(ctrFile, netNs)
}

// Retrieve a container file from disk
func (s *FileState) getContainerFileFromDisk(id, sandboxID string) (*containerFile, error) {
	ctrExists, err := s.checkContainerExistsOnDisk(id, sandboxID)
	if err != nil {
		return nil, err
	} else if !ctrExists {
		return nil, fmt.Errorf("container with ID %v in sandbox %v does not exist", id, sandboxID)
	}

	ctrPath := s.getContainerPath(id, sandboxID)
	ctrFile := containerFile{}

	if err := decodeFromFile(ctrPath, &ctrFile); err != nil {
		return nil, fmt.Errorf("error retrieving containder ID %v from disk: %v", id, err)
	}

	return &ctrFile, nil
}

// Store a container on disk
// Cowardly refuses to replace containers that already exist on disk
// If parameter is set to true, will also update associated sandbox with new container
func (s *FileState) putContainerToDisk(ctr *oci.Container, updateSandbox bool) error {
	ctrFile := getContainerFileFromContainer(ctr)

	if updateSandbox {
		sbFile, err := s.getSandboxFileFromDisk(ctrFile.Sandbox)
		if err != nil {
			return err
		}

		sbFile.Containers = append(sbFile.Containers, ctrFile.ID)

		if err := s.updateSandboxFileOnDisk(sbFile); err != nil {
			return err
		}
	}

	return s.putContainerFileToDisk(ctrFile)
}

// Put a container file to disk
// Will throw an error if a container with that ID already exists on disk
// Does not update associated sandbox
func (s *FileState) putContainerFileToDisk(ctrFile *containerFile) error {
	ctrExists, err := s.checkContainerExistsOnDisk(ctrFile.ID, ctrFile.Sandbox)
	if err != nil {
		return err
	} else if ctrExists {
		return fmt.Errorf("container with ID %v already exists on disk, cowardly refusing to replace", ctrFile.ID)
	}

	ctrPath := s.getContainerPath(ctrFile.ID, ctrFile.Sandbox)

	if err := encodeToFile(ctrPath, ctrFile); err != nil {
		return fmt.Errorf("error storing container with ID %v: %v", ctrFile.ID, err)
	}

	if err := s.lockfile.Touch(); err != nil {
		logrus.Errorf("error updating lockfile writer: %v", err)
	}

	return nil
}

// Remove a container from disk, updating sandbox to remove references to it
func (s *FileState) removeContainerFromDisk(id, sandboxID string) error {
	ctrExists, err := s.checkContainerExistsOnDisk(id, sandboxID)
	if err != nil {
		return err
	} else if !ctrExists {
		return fmt.Errorf("cannot remove container ID %v from sandbox %v as it does not exist", id, sandboxID)
	}

	// Load, update, and store the sandbox descriptor file to reflect removed container
	sbFile, err := s.getSandboxFileFromDisk(sandboxID)
	if err != nil {
		return err
	}

	foundID := false
	newCtrs := make([]string, 0, len(sbFile.Containers))
	for _, ctrID := range sbFile.Containers {
		if ctrID == id {
			foundID = true
		} else {
			newCtrs = append(newCtrs, ctrID)
		}
	}

	if !foundID {
		return fmt.Errorf("error updating sandbox %v to remove container %v: container not found in sandbox containers listing", sandboxID, id)
	}

	sbFile.Containers = newCtrs

	if err := s.updateSandboxFileOnDisk(sbFile); err != nil {
		return err
	}

	// Now remove container file
	ctrPath := s.getContainerPath(id, sandboxID)

	if err := os.Remove(ctrPath); err != nil {
		return fmt.Errorf("error removing container %v in sandbox %v: %v", id, sandboxID, err)
	}

	return nil
}

// Check if given container exists in given sandbox
func (s *FileState) checkContainerExistsOnDisk(id, sandboxID string) (bool, error) {
	sbExists, err := s.checkSandboxExistsOnDisk(sandboxID)
	if err != nil {
		return false, fmt.Errorf("error checking sandbox %v: %v", sandboxID, err)
	} else if !sbExists {
		return false, nil
	}

	ctrPath := s.getContainerPath(id, sandboxID)
	if _, err := os.Stat(ctrPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat error on container file %v: %v", ctrPath, err)
	}

	return true, nil
}

// Get path of file representing a single container
func (s *FileState) getContainerPath(id, sandboxID string) string {
	return path.Join(s.rootPath, sandboxID, (id + ".json"))
}

// Encode given struct into a file with the given name
// Will refuse to replace files that already exist
func encodeToFile(fileName string, toEncode interface{}) error {
	// Open file for writing
	file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("could not open file %v for writing: %v", fileName, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "")
	if err := encoder.Encode(toEncode); err != nil {
		return fmt.Errorf("error encoding & storing struct: %v", err)
	}

	return nil
}

// Decode a single JSON structure (if multiple are present, the first will be used) from given file into given struct
func decodeFromFile(fileName string, decodeInto interface{}) error {
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("error opening data file %v: %v", fileName, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(decodeInto); err != nil {
		return fmt.Errorf("error decoding contents of file %v: %v", fileName, err)
	}

	return nil
}

// Public API

// NewFileState makes a new file-based state store at the given directory
// TODO: Should we attempt to populate the state based on the directory that exists,
// or should we let server's sync() handle that?
func NewFileState(statePath string) (StateStore, error) {
	state := new(FileState)
	state.rootPath = statePath
	state.memoryState = NewInMemoryState()

	// Make the root path if it does not exist
	pathStat, err := os.Stat(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			if err2 := os.Mkdir(statePath, 0700); err2 != nil {
				return nil, fmt.Errorf("unable to make root path directory %v: %v", statePath, err2)
			}
		} else {
			return nil, fmt.Errorf("unable to stat root path of state: %v", err)
		}
	} else if !pathStat.IsDir() {
		return nil, fmt.Errorf("root path %v already exists and is not a directory", statePath)
	}

	// Retrieve the lockfile
	lockfilePath := path.Join(statePath, "STATE_LOCK")

	lockfile, err := storage.GetLockfile(lockfilePath)
	if err != nil {
		return nil, fmt.Errorf("error retrieving lock: %v", err)
	}

	state.lockfile = lockfile

	state.lockfile.Lock()
	defer state.lockfile.Unlock()

	// Check if the lockfile is fresh
	// If it is (Modified returns ENOSPC as there is no writer present), make us the writer
	if _, err := lockfile.Modified(); err != nil {
		if err == syscall.ENOSPC {
			if err2 := lockfile.Touch(); err2 != nil {
				return nil, fmt.Errorf("error adding writer to lockfile :%v", err2)
			}
		} else {
			return nil, fmt.Errorf("error checking if lockfile modified: %v", err)
		}
	}

	// Perform an initial sync with the disk
	// Should be a no-op if we set ourself as the writer above
	if err := state.syncWithDisk(); err != nil {
		return nil, fmt.Errorf("error syncing on-disk state: %v", err)
	}

	return state, nil
}

// AddSandbox adds a sandbox and any containers in it to the state
func (s *FileState) AddSandbox(sb *sandbox) error {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	if s.memoryState.HasSandbox(sb.id) {
		return fmt.Errorf("sandbox with ID %v already exists", sb.id)
	}

	if err := s.putSandboxToDisk(sb); err != nil {
		return err
	}

	if err := s.memoryState.AddSandbox(sb); err != nil {
		return fmt.Errorf("error adding sandbox %v to in-memory state: %v", sb.id, err)
	}

	return nil
}

// HasSandbox checks if a sandbox exists in the state
func (s *FileState) HasSandbox(id string) bool {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	// TODO: maybe this function should return an error so we can better handle this?
	if err := s.syncWithDisk(); err != nil {
		return false
	}

	return s.memoryState.HasSandbox(id)
}

// DeleteSandbox removes the given sandbox from the state
func (s *FileState) DeleteSandbox(id string) error {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	if !s.memoryState.HasSandbox(id) {
		return fmt.Errorf("cannot remove sandbox %v as it does not exist", id)
	}

	if err := s.removeSandboxFromDisk(id); err != nil {
		return err
	}

	if err := s.memoryState.DeleteSandbox(id); err != nil {
		return fmt.Errorf("error removing sandbox %v from in-memory state: %v", id, err)
	}

	return nil
}

// GetSandbox retrieves the given sandbox from the state
func (s *FileState) GetSandbox(id string) (*sandbox, error) {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return nil, fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	return s.memoryState.GetSandbox(id)
}

// LookupSandboxByName returns a sandbox given its full or partial name
func (s *FileState) LookupSandboxByName(name string) (*sandbox, error) {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return nil, fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	return s.memoryState.LookupSandboxByName(name)
}

// LookupSandboxByID returns a sandbox given its full or partial ID
// An error will be returned if the partial ID given is not unique
func (s *FileState) LookupSandboxByID(id string) (*sandbox, error) {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return nil, fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	return s.memoryState.LookupSandboxByID(id)
}

// GetAllSandboxes returns all sandboxes in the state
func (s *FileState) GetAllSandboxes() ([]*sandbox, error) {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return nil, fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	return s.memoryState.GetAllSandboxes()
}

// AddContainer adds a container to the state
func (s *FileState) AddContainer(c *oci.Container, sandboxID string) error {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	if s.memoryState.HasContainer(c.ID(), sandboxID) {
		return fmt.Errorf("container with id %v in sandbox %v already exists", c.ID(), sandboxID)
	}

	if err := s.putContainerToDisk(c, true); err != nil {
		return err
	}

	if err := s.memoryState.AddContainer(c, sandboxID); err != nil {
		return fmt.Errorf("error adding container %v to in-memory state: %v", c.ID(), err)
	}

	return nil
}

// HasContainer checks if a container exists in a given sandbox
func (s *FileState) HasContainer(id, sandboxID string) bool {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	// TODO: Should return (bool, error) to better represent this error? Sync failure is serious
	if err := s.syncWithDisk(); err != nil {
		return false
	}

	return s.memoryState.HasContainer(id, sandboxID)
}

// DeleteContainer removes a container from a given sandbox in the state
func (s *FileState) DeleteContainer(id, sandboxID string) error {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	if !s.memoryState.HasContainer(id, sandboxID) {
		return fmt.Errorf("cannot remove container %v in sandbox %v as it does not exist", id, sandboxID)
	}

	if err := s.removeContainerFromDisk(id, sandboxID); err != nil {
		return err
	}

	if err := s.memoryState.DeleteContainer(id, sandboxID); err != nil {
		return fmt.Errorf("error removing container %v from in-memory state: %v", id, err)
	}

	return nil
}

// GetContainer retrieves the container with given ID from the given sandbox
func (s *FileState) GetContainer(id, sandboxID string) (*oci.Container, error) {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return nil, fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	return s.memoryState.GetContainer(id, sandboxID)
}

// GetContainerSandbox retrieves the sandbox of the container with given ID
func (s *FileState) GetContainerSandbox(id string) (string, error) {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return "", fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	return s.memoryState.GetContainerSandbox(id)
}

// LookupContainerByName returns the full ID of a container given its full or partial name
func (s *FileState) LookupContainerByName(name string) (*oci.Container, error) {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return nil, fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	return s.memoryState.LookupContainerByName(name)
}

// LookupContainerByID returns the full ID of a container given a full or partial ID
// If the given ID is not unique, an error is returned
func (s *FileState) LookupContainerByID(id string) (*oci.Container, error) {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return nil, fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	return s.memoryState.LookupContainerByID(id)
}

// GetAllContainers returns all containers in the state, regardless of which sandbox they belong to
// Pod Infra containers are not included
func (s *FileState) GetAllContainers() ([]*oci.Container, error) {
	s.lockfile.Lock()
	defer s.lockfile.Unlock()

	if err := s.syncWithDisk(); err != nil {
		return nil, fmt.Errorf("error syncing with on-disk state: %v", err)
	}

	return s.memoryState.GetAllContainers()
}
