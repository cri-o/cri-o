// +build linux

package lvm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/storage/drivers"
	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/ioutils"
	"github.com/containers/storage/pkg/mount"
	"github.com/containers/storage/pkg/stringid"
	units "github.com/docker/go-units"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	defaultVGname       = "containers"
	defaultPoolName     = "loopbackpool"
	defaultLoopbackFile = "loopbackfile"
	defaultFS           = "xfs"
)

type lvmDriver struct {
	root         string
	vgname       string
	poolname     string
	fs           string
	loopbackFile string
	sparse       bool
	ctr          *graphdriver.RefCounter
}

// this is our record of the last pool that we used, along with its UUID.  We
// need to error out if it's changed, because the set of layers the pool has
// likely no longer matches what higher level APIs think we have.
type lvmPoolHistory struct {
	VGname   string `json:"vgname"`
	PoolName string `json:"poolname"`
	PoolUUID string `json:"uuid"`
}

// generateID returns a new sufficiently-unique identifier which we can assign
// to a layer that doesn't have one specified for it when we're asked to create
// a new layer.
func generateID() string {
	return stringid.GenerateRandomID()
}

// volumeNameForID converts an ID into a reasonable volume name.
func volumeNameForID(ID string) string {
	return "layer." + ID
}

// mountPathForID generates a path name that we should use as the mountpoint
// for a volume with the specified ID.
func mountPathForID(root, id string) string {
	if id == "" {
		return filepath.Join(root, "mounts")
	}
	return filepath.Join(root, "mounts", id)
}

// compute the location of the pool info file
func poolInfoPathForRoot(root string) string {
	return filepath.Join(root, "pool-info.json")
}

// read information about the pool we were using before
func readPoolHistory(root string) (lvmPoolHistory, error) {
	logrus.Debugf("reading pool history info from file %q", poolInfoPathForRoot(root))
	b, err := ioutil.ReadFile(poolInfoPathForRoot(root))
	if err != nil {
		return lvmPoolHistory{}, errors.Wrapf(err, "error reading pool info from file %q", poolInfoPathForRoot(root))
	}
	history := lvmPoolHistory{}
	err = json.Unmarshal(b, &history)
	if err != nil {
		return lvmPoolHistory{}, errors.Wrapf(err, "error parsing pool info from file %q", poolInfoPathForRoot(root))
	}
	return history, nil
}

// write information about the pool we're using now
func writePoolInfo(root string, info lvmPoolHistory) error {
	logrus.Debugf("writing pool history info to file %q", poolInfoPathForRoot(root))
	b, err := json.Marshal(&info)
	if err != nil {
		return errors.Wrapf(err, "error encoding pool info")
	}
	err = ioutils.AtomicWriteFile(poolInfoPathForRoot(root), b, 0600)
	if err != nil {
		return errors.Wrapf(err, "error saving pool info to file %q", poolInfoPathForRoot(root))
	}
	return nil
}

// ensureLoopbackFileExists checks if there's a file with the specified name.
// If it's not found, it's created with the specified size, either as a sparse
// file or filled with zeroes.
func ensureLoopbackFileExists(filename string, createSize int64, sparse bool) error {
	logrus.Debugf("checking if file %q exists", filename)
	_, err := os.Stat(filename)
	if err != nil && os.IsNotExist(err) {
		if sparse {
			logrus.Debugf("file %q does not exist, creating it as a sparse file", filename)
			fd, err2 := unix.Open(filename, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
			if err2 != nil {
				return errors.Wrapf(err, "error creating file %q", filename)
			}
			unix.CloseOnExec(fd)
			err = unix.Ftruncate(fd, createSize)
			unix.Close(fd)
			if err != nil {
				return errors.Wrapf(err, "error expanding file %q", filename)
			}
			logrus.Debugf("created sparse loopback file %q", filename)
		} else {
			logrus.Debugf("file %q does not exist, creating it as a zero-filled file", filename)
			fd, err2 := unix.Open(filename, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
			if err2 != nil {
				return errors.Wrapf(err, "error creating file %q", filename)
			}
			unix.CloseOnExec(fd)
			err = unix.Fallocate(fd, 0, 0, createSize)
			unix.Close(fd)
			if err != nil {
				return errors.Wrapf(err, "error allocating space for file %q", filename)
			}
			logrus.Debugf("created non-sparse loopback file %q", filename)
		}
	}
	if err != nil {
		return errors.Wrapf(err, "error examining file %q", filename)
	}
	return nil
}

// vgnameForFile computes a unique volume group name based on the file's device and inode number.  Its main purpose is
// to keep different loopback files from interfering with each other by appearing to have the same volume group name
// and thin pool name.
func vgnameForFile(loopbackFile string) (string, error) {
	var st unix.Stat_t
	err := unix.Stat(loopbackFile, &st)
	if err != nil {
		return "", errors.Wrapf(err, "error examining file %q", loopbackFile)
	}
	vgname := fmt.Sprintf("containers-loopback-%x.%d", st.Dev, st.Ino)
	return vgname, nil
}

// createAndActivateVolumeGroupOnLoopback creates a volume group using loopback devices backed by files.
func createAndActivateVolumeGroupOnLoopback(lvmRoot, loopbackFile string, loopbackSize int64, sparse bool, vgname string) (string, error) {
	if loopbackFile == "" {
		loopbackFile = defaultLoopbackFile
	}
	if !filepath.IsAbs(loopbackFile) {
		loopbackFile = filepath.Join(lvmRoot, loopbackFile)
	}
	loopbackDir := filepath.Dir(loopbackFile)
	err := os.MkdirAll(loopbackDir, 0700)
	if err != nil {
		return vgname, errors.Wrapf(err, "error ensuring directory %q exists", loopbackDir)
	}

	logrus.Debugf("checking for loopback file %q", loopbackFile)
	err = ensureLoopbackFileExists(loopbackFile, loopbackSize, sparse)
	if err != nil {
		return vgname, errors.Wrapf(err, "error ensuring loopback file %q exists", loopbackFile)
	}
	vgname, err = vgnameForFile(loopbackFile)
	if err != nil {
		return vgname, errors.Wrapf(err, "error computing volume name for loopback file %q", loopbackFile)
	}

	logrus.Debugf("attaching loopback file %q", loopbackFile)
	loopbackDevice, err := startLoopbackDeviceOnFile(loopbackFile)
	if err != nil {
		return vgname, errors.Wrapf(err, "error starting loopback device for loopback file %q", loopbackFile)
	}
	logrus.Debugf("attached loopback device %q for loopback file %q", loopbackDevice, loopbackFile)

	err = resizeLoopbackDevice(loopbackDevice)
	if err != nil {
		return vgname, errors.Wrapf(err, "error checking if loopback device %q was resized", loopbackDevice)
	}

	logrus.Debugf("checking for physical volume header information on device %q", loopbackDevice)
	if !physicalVolumeIsPresent(loopbackDevice) {
		logrus.Debugf("formatting device %q as a new physical volume", loopbackDevice)
		err = createPhysicalVolume(loopbackDevice)
		if err != nil {
			return vgname, errors.Wrapf(err, "error creating LVM PV on %q", loopbackDevice)
		}
		logrus.Debugf("created physical volume on device %q", loopbackDevice)
		err = resizePhysicalVolume(loopbackDevice)
		if err != nil {
			return vgname, errors.Wrapf(err, "error checking if LVM PV on device %q has changed in size", loopbackDevice)
		}
		logrus.Debugf("creating volume group %q using physical volume %q", vgname, loopbackDevice)
		err = createVolumeGroup(vgname, loopbackDevice)
		if err != nil {
			return vgname, errors.Wrapf(err, "error creating LVM VG using device %q", loopbackDevice)
		}
		logrus.Debugf("created volume group %q using physical volume %q", vgname, loopbackDevice)
	} else {
		err = resizePhysicalVolume(loopbackDevice)
		if err != nil {
			return vgname, errors.Wrapf(err, "error checking if LVM PV on device %q has changed in size", loopbackDevice)
		}
		logrus.Debugf("reading volume group name from physical volume %q", loopbackDevice)
		pvvgname, err := readVolumeGroupForPhysicalVolume(loopbackDevice)
		if err != nil {
			return vgname, errors.Wrapf(err, "error reading VG of LVM PV on device %q", loopbackDevice)
		}
		if pvvgname == "" {
			logrus.Debugf("physical volume %q exists but was not in a volume group", loopbackDevice)
			err = createVolumeGroup(vgname, loopbackDevice)
			if err != nil {
				return vgname, errors.Wrapf(err, "error creating LVM VG using physical volume %q", loopbackDevice)
			}
			logrus.Debugf("created volume group %q using physical volume %q", vgname, loopbackDevice)
			pvvgname = vgname
		} else {
			logrus.Debugf("using volume group name %q", pvvgname)
			vgname = pvvgname
			logrus.Debugf("found volume group %q containing physical volume %q", vgname, loopbackDevice)
		}
	}

	logrus.Debugf("marking volume group %q active", vgname)
	err = activateVolumeGroup(vgname)
	if err != nil {
		return "", errors.Wrapf(err, "error activating volume group %q", vgname)
	}
	logrus.Debugf("activated volume group %q", vgname)
	return vgname, nil
}

// createAndActivateThinPoolInVolumeGroup creates a thin pool in the specified volume group.
func createAndActivateThinPoolInVolumeGroup(vgname, poolname string) error {
	report, err := getVolumeGroups(vgname)
	if err != nil {
		return errors.Wrapf(err, "error reading information about volume group %q", vgname)
	}
	size := int64(-1)
	free := int64(-1)
	for _, entry := range report.Reports {
		for _, vg := range entry.VGs {
			if vg.Name == vgname {
				size = vg.Size
				free = vg.Free
			}
		}
	}
	if size < 0 {
		return errors.Errorf("unable to read information about volume group %q", vgname)
	}
	logrus.Debugf("volume group %q has %d bytes free", vgname, free)
	if free < 64*1024*1024 {
		return errors.Errorf("not enough free space in volume group %q", vgname)
	}
	poolSize := int64(0)
	metaSize := int64(0)
	if free < 12*1024*1024*1024 {
		poolSize = free * 8 / 10
		metaSize = poolSize / 10
	} else {
		poolSize = 10 * 1024 * 1024 * 1024
		metaSize = poolSize / 10
	}
	logrus.Debugf("creating data logical volume with size %d bytes, metadata logical volume with size %d bytes", poolSize, metaSize)
	initialMetadataName := poolname + "-tmeta"
	logrus.Debugf("creating data logical volume with size %d bytes", poolSize)
	err = runWithoutOutput(LVMPath, "lvcreate", "--size", fmt.Sprintf("%dK", poolSize/1024), "--name", poolname, vgname)
	if err != nil {
		return errors.Wrapf(err, "error creating logical volume %q", vgname+"/"+poolname)
	}
	logrus.Debugf("creating metadata logical volume with size %d bytes", metaSize)
	err = runWithoutOutput(LVMPath, "lvcreate", "--size", fmt.Sprintf("%dK", metaSize/1024), "--name", initialMetadataName, vgname)
	if err != nil {
		return errors.Wrapf(err, "error creating logical volume %q", vgname+"/"+initialMetadataName)
	}
	logrus.Debugf("creating thin pool logical volume using data logical volume %q and metadata logical volume %q", poolname, initialMetadataName)
	err = runWithoutOutput(LVMPath, "lvconvert", "--yes", "--type", "thin-pool", "--poolmetadata", vgname+"/"+initialMetadataName, vgname+"/"+poolname)
	if err != nil {
		return errors.Wrapf(err, "error creating thin pool logical volume %q", vgname+"/"+poolname)
	}
	logrus.Debugf("checking for thin pool logical volume %q", poolname)
	if !logicalVolumeIsPresent(vgname, poolname) {
		return errors.Errorf("error creating thin pool %q: pool not visible after it was created", vgname+"/"+poolname)
	}
	return nil
}

func initLVM(root string, options []string, uidMaps, gidMaps []idtools.IDMap) (graphdriver.Driver, error) {
	if LVMPath == "" {
		return nil, errors.Errorf("error locating \"lvm\" command")
	}
	if LosetupPath == "" {
		return nil, errors.Errorf("error locating \"losetup\" command")
	}
	// name of the volume group which contains the pool
	vgname := ""
	// name of the thinpool
	poolname := ""
	// type of filesystem to use for the layer devices
	fs := ""
	// name of the main loopback file to use
	loopbackFile := ""
	// size of the loopback file to create, if we need to create one
	loopbackSize := int64(10 * 1024 * 1024 * 1024)
	// whether or not the loopback file, if we create it, should be sparse
	sparse := true
	// parse values for these settings from our passed-in options list
	for _, opt := range options {
		// Name of the volume group to use.  If it does not exist,
		// we'll create one using loopback files.
		if strings.HasPrefix(opt, "lvm.vg=") {
			vgname = opt[7:]
			continue
		}
		// Name of the thin pool volume in the volume group.  If it
		// does not exist, we'll create one with that name.
		if strings.HasPrefix(opt, "lvm.pool=") {
			poolname = opt[9:]
			continue
		}
		// Type of filesystem to use for layers.  If not specified,
		// we'll select a default.
		if strings.HasPrefix(opt, "lvm.fs=") {
			fs = opt[7:]
			continue
		}
		// Location of a loopback file to use as backing, if we end up
		// creating a volume group using loopback files.
		if strings.HasPrefix(opt, "lvm.loopback=") {
			loopbackFile = opt[13:]
			continue
		}
		// Size of the loopback file to create, if we end up creating a
		// volume group using loopback files and the file doesn't exist
		// yet.
		if strings.HasPrefix(opt, "lvm.loopbacksize=") {
			spec, err := units.FromHumanSize(opt[17:])
			if err != nil {
				return nil, errors.Wrapf(err, "error parsing loopback size %q", opt[17:])
			}
			loopbackSize = spec
			continue
		}
		// Whether or not we create a loopback file as a sparse file.
		if strings.HasPrefix(opt, "lvm.sparse=") {
			sparseFlag := opt[11:]
			if strings.ToLower(sparseFlag) == "false" {
				sparse = false
			} else if strings.ToLower(sparseFlag) == "true" {
				sparse = true
			} else {
				return nil, errors.Errorf("error parsing boolean value of lvm.loopback: %q", sparseFlag)
			}
			continue
		}
		return nil, errors.Errorf("unsupported option %q", opt)
	}
	if fs == "" {
		fs = defaultFS
	}
	neededButMissing := FSNeededCommands(fs)
	if len(neededButMissing) > 0 {
		return nil, errors.Errorf("missing required command %q for dealing with %q filesystems", neededButMissing, fs)
	}
	lvmRoot := root
	err := os.MkdirAll(lvmRoot, 0700)
	if err != nil {
		return nil, errors.Wrapf(err, "error ensuring directory %q exists", lvmRoot)
	}
	err = mount.MakePrivate(lvmRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "error ensuring directory %q is mounted privately", lvmRoot)
	}
	if vgname == "" {
		vgname = defaultVGname
	}
	if !volumeGroupIsPresent(vgname) {
		logrus.Debugf("volume group %q not found, creating it using loopback devices", vgname)
		if vgname, err = createAndActivateVolumeGroupOnLoopback(lvmRoot, loopbackFile, loopbackSize, sparse, vgname); err != nil {
			return nil, errors.Wrapf(err, "error creating and activating volume group %q", vgname)
		}
	} else {
		logrus.Debugf("volume group %q was already activated", vgname)
	}
	if poolname == "" {
		poolname = defaultPoolName
	}
	if logicalVolumeIsPresent(vgname, poolname) {
		logrus.Debugf("activating thin pool logical volume %q in group %q", poolname, vgname)
		if err = activateLogicalVolume(vgname, poolname); err != nil {
			return nil, errors.Wrapf(err, "error activating logical volume %q in group %q", poolname, vgname)
		}
		logrus.Debugf("activated thin pool logical volume %q in group %q", poolname, vgname)
	} else {
		logrus.Debugf("thin pool logical volume %q not found in group %q, creating it", poolname, vgname)
		if err = createAndActivateThinPoolInVolumeGroup(vgname, poolname); err != nil {
			return nil, errors.Wrapf(err, "error creating and activating logical volume %q in group %q", poolname, vgname)
		}
		logrus.Debugf("created and activated thin pool %q in group %q", poolname, vgname)
	}
	current, err := readPoolInfo(vgname, poolname)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading information about thin pool %q in group %q", poolname, vgname)
	}
	if history, err := readPoolHistory(root); err == nil {
		if history.PoolUUID != current.PoolUUID {
			return nil, errors.Errorf("we've been using a pool with UUID %q, but this one has UUID %q, which is unexpected", history.PoolUUID, current.PoolUUID)
		}
	} else {
		err = writePoolInfo(root, current)
		if err != nil {
			return nil, errors.Wrapf(err, "error saving information about thin pool %q in group %q", poolname, vgname)
		}
	}
	driver := &lvmDriver{
		root:         root,
		vgname:       vgname,
		poolname:     poolname,
		fs:           fs,
		loopbackFile: loopbackFile,
		sparse:       sparse,
		ctr:          graphdriver.NewRefCounter(graphdriver.NewDefaultChecker()),
	}
	return graphdriver.NewNaiveDiffDriver(driver, graphdriver.NewNaiveLayerIDMapUpdater(driver)), nil
}

func init() {
	graphdriver.Register("lvm", initLVM)
}

// String returns the driver's name ("lvm") as a string.
func (l *lvmDriver) String() string {
	return "lvm"
}

// Status returns a set of key-value pairs which give diagnostic status about
// this driver instance.
func (l *lvmDriver) Status() [][2]string {
	status := [][2]string{
		{"vg_name", l.vgname},
		{"lv_name", l.poolname},
	}
	return status
}

// createSnapshot creates a new read-only or read-write filesystem layer that
// is initially a duplicate of another layer.  The permissions are either "r"
// or "rw", indicating read-only, or read-write, respectively.
func (l *lvmDriver) createSnapshot(id, parent, mountLabel, permissions string, storageOpt map[string]string) error {
	volume := volumeNameForID(id)
	parentVolume := volumeNameForID(parent)
	logrus.Debugf("creating logical volume %q based on logical volume %q", volume, parentVolume)
	err := runWithoutOutput(LVMPath, "lvcreate", "--name", volume, "--addtag", "@layer", "--setactivationskip", "y", "--ignoreactivationskip", "--activate", "y", "--permission", permissions, "--snapshot", l.vgname+"/"+parentVolume)
	if err != nil {
		return errors.Wrapf(err, "error creating snapshot logical volume %q from logical volume %q for %q", volume, parentVolume, id)
	}
	logrus.Debugf("looking for device for logical volume %q", volume)
	lvDevice, err := volumePathForID(l.vgname, id)
	if err != nil {
		return errors.Wrapf(err, "error locating just-created snapshot logical volume %q for ID %q", volume, id)
	}
	cmd := FSPostSnapshotCmd(l.fs, lvDevice)
	logrus.Debugf("running post-create command %v", cmd)
	err = runWithoutOutput(cmd[0], cmd[1:]...)
	if err != nil {
		return errors.Wrapf(err, "error running post-create command %v", cmd)
	}
	logrus.Debugf("deactivating logical volume %q", volume)
	err = deactivateLogicalVolume(l.vgname, volume)
	if err != nil {
		return errors.Wrapf(err, "error deactivating just-created logical volume %q for ID %q", volume, id)
	}
	return nil
}

// createBase creates an empty filesystem layer.  It has to be writeable so
// that we can put a filesystem on it.
func (l *lvmDriver) createBase(id, mountLabel string, storageOpt map[string]string) error {
	report, err := getLogicalVolume(l.vgname, l.poolname)
	if err != nil {
		return errors.Wrapf(err, "error reading information about thin pool logical volume %q", l.vgname+"/"+l.poolname)
	}
	virtualSize := report.Size
	volume := volumeNameForID(id)
	logrus.Debugf("creating new logical volume %q", volume)
	err = runWithoutOutput(LVMPath, "lvcreate", "--activate", "y", "--name", volume, "--addtag", "@layer", "--virtualsize", fmt.Sprintf("%dB", virtualSize), "--setactivationskip", "y", "--ignoreactivationskip", "--thinpool", l.vgname+"/"+l.poolname)
	if err != nil {
		return errors.Wrapf(err, "error creating logical volume %q for ID %q", volume, id)
	}
	logrus.Debugf("looking for device for logical volume %q", volume)
	lvDevice, err := volumePathForID(l.vgname, id)
	if err != nil {
		return errors.Wrapf(err, "error locating just-created logical volume %q for ID %q", volume, id)
	}
	logrus.Debugf("formatting device %q as %q", lvDevice, l.fs)
	err = runWithoutOutput("mkfs", "-t", l.fs, lvDevice)
	if err != nil {
		return errors.Wrapf(err, "error formatting just-created logical volume %q as %q", volume, l.fs)
	}
	logrus.Debugf("deactivating logical volume %q", volume)
	err = deactivateLogicalVolume(l.vgname, volume)
	if err != nil {
		return errors.Wrapf(err, "error deactivating just-created logical volume %q for ID %q", volume, id)
	}
	return nil
}

// creates a new filesystem layer that is either empty or begins as a snapshot
// of another layer.
func (l *lvmDriver) create(id, parent, mountLabel, permissions string, storageOpt map[string]string) error {
	if id == "" {
		id = generateID()
	}
	if parent != "" {
		err := l.createSnapshot(id, parent, mountLabel, permissions, storageOpt)
		if err != nil {
			return errors.Wrapf(err, "error creating snapshot logical volume for ID %q from logical volume for ID %q", id, parent)
		}
		return nil
	}
	err := l.createBase(id, mountLabel, storageOpt)
	if err != nil {
		return errors.Wrapf(err, "error creating new logical volume for ID %q", id)
	}
	return nil
}

// CreateReadWrite creates a new, empty writeable filesystem layer that is
// ready to be used as the storage for a container.
func (l *lvmDriver) CreateReadWrite(id, parent string, options *graphdriver.CreateOpts) error {
	if options == nil {
		options = &graphdriver.CreateOpts{}
	}
	logrus.Debugf("creating read-write logical volume for ID %q", id)
	if err := l.create(id, parent, options.MountLabel, "rw", options.StorageOpt); err != nil {
		return errors.Wrapf(err, "error creating read-write logical logical volume for ID %q", id)
	}
	return nil
}

// Create creates a new, empty, read only filesystem layer with the specified
// id and parent and mountLabel. Parent and mountLabel may be "".
func (l *lvmDriver) Create(id, parent string, options *graphdriver.CreateOpts) error {
	if options == nil {
		options = &graphdriver.CreateOpts{}
	}
	logrus.Debugf("creating \"read-only\" logical volume for ID %q", id)
	if err := l.create(id, parent, options.MountLabel, "rw", options.StorageOpt); err != nil {
		return errors.Wrapf(err, "error creating \"read-only\" logical volume for ID %q", id)
	}
	return nil
}

// CreateFromTemplate creates a new filesystem layer with the specified id and
// parent, with contents matching the specified template layer, and with the
// specified mountLabel. Parent and mountLabel may be "".
func (l *lvmDriver) CreateFromTemplate(id, template string, templateMappings *idtools.IDMappings, parent string, parentMappings *idtools.IDMappings, options *graphdriver.CreateOpts, readWrite bool) error {
	if options == nil {
		options = &graphdriver.CreateOpts{}
	}
	logrus.Debugf("creating logical volume for ID %q based on %q", id, template)
	if err := l.create(id, template, options.MountLabel, "rw", options.StorageOpt); err != nil {
		return errors.Wrapf(err, "error creating logical volume for ID %q based on %q", id, template)
	}
	return nil
}

// Remove attempts to remove the filesystem layer with this id.
func (l *lvmDriver) Remove(id string) error {
	logrus.Debugf("removing logical volume for ID %q", id)
	volume := volumeNameForID(id)
	err := deactivateLogicalVolume(l.vgname, volume)
	if err != nil {
		return errors.Wrapf(err, "error deactivating logical volume %q for ID %q", volume, id)
	}
	err = runWithoutOutput(LVMPath, "lvremove", l.vgname+"/"+volume)
	if err != nil {
		return errors.Wrapf(err, "error removing logical volume %q for ID %q", volume, id)
	}
	return nil
}

// Get returns the mountpoint for the layered filesystem referred to by this
// id. You can optionally specify a mountLabel or "".  Returns the absolute
// path to the mounted layered filesystem.
func (l *lvmDriver) Get(id string, options graphdriver.MountOpts) (dir string, err error) {
	volume := volumeNameForID(id)
	mpath := mountPathForID(l.root, id)
	if count := l.ctr.Increment(mpath); count > 1 {
		logrus.Debugf("taking reference on %q", mpath)
		return mpath, nil
	}
	logrus.Debugf("mounting logical volume %q for ID %q at %q", volume, id, mpath)
	defer func() {
		if err != nil {
			logrus.Debugf("re-decrementing reference on %q due to %v", mpath, err)
			l.ctr.Decrement(mpath)
		}
	}()
	logrus.Debugf("activating logical volume %q", volume)
	err = activateLogicalVolume(l.vgname, volume)
	if err != nil {
		return "", errors.Wrapf(err, "error activating logical volume %q for ID %q", volume, id)
	}
	err = os.MkdirAll(mpath, 0700)
	if err != nil {
		return "", errors.Wrapf(err, "error ensuring mountpoint directory %q exists", mpath)
	}
	logrus.Debugf("looking up device name for logical volume %q", volume)
	lvDevice, err := volumePathForID(l.vgname, id)
	if err != nil {
		return "", errors.Wrapf(err, "error finding device path for logical volume %q", volume)
	}
	logrus.Debugf("mounting volume device %q on %q", lvDevice, mpath)
	err = mount.Mount(lvDevice, mpath, l.fs, label.FormatMountLabel(FSMountOptions(l.fs, lvDevice), options.MountLabel))
	if err != nil {
		return "", errors.Wrapf(err, "error mounting logical volume device %q for ID %q at %q", lvDevice, id, mpath)
	}
	return mpath, nil
}

// Put releases the system resources for the specified id, i.e., it unmounts
// the layered filesystem.
func (l *lvmDriver) Put(id string) (err error) {
	volume := volumeNameForID(id)
	mpath := mountPathForID(l.root, id)
	if count := l.ctr.Decrement(mpath); count > 0 {
		logrus.Debugf("dropping reference on %q", mpath)
		return nil
	}
	logrus.Debugf("unmounting logical volume %q for ID %q from %q", volume, id, mpath)
	err = mount.Unmount(mpath)
	if err != nil && !os.IsNotExist(err) && err != unix.EINVAL {
		return errors.Wrapf(err, "error unmounting %q", mpath)
	}
	logrus.Debugf("removing mountpoint directory %q", mpath)
	err = os.Remove(mpath)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "error removing mountpoint for logical volume %q at %q", volume, mpath)
	}
	logrus.Debugf("deactivating logical volume %q", volume)
	err = deactivateLogicalVolume(l.vgname, volume)
	if err != nil {
		return errors.Wrapf(err, "error deactivating logical volume %q for ID %q", volume, id)
	}
	return nil
}

// Exists returns whether a filesystem layer with the specified ID exists, so
// far as the driver knows.
func (l *lvmDriver) Exists(id string) bool {
	logrus.Debugf("checking if there is a logical volume for ID %q", id)
	return logicalVolumeIsPresent(l.vgname, volumeNameForID(id))
}

// Returns a set of key-value pairs which give low level information about the
// image/container driver is managing.
func (l *lvmDriver) Metadata(id string) (map[string]string, error) {
	status := map[string]string{
		"Volume Group":       l.vgname,
		"Pool Name":          l.poolname,
		"Backing Filesystem": l.fs,
	}
	if len(l.loopbackFile) > 0 {
		status["Loopback file"] = l.loopbackFile
	}
	return status, nil
}

// Cleanup performs necessary tasks to release resources held by the driver,
// e.g., deactivating unused devices.
func (l *lvmDriver) Cleanup() error {
	report, err := getPhysicalVolumes("")
	if err != nil {
		return errors.Wrap(err, "error getting list of physical volumes")
	}
	loopbacks, err := getLoopbacks()
	if err != nil {
		return errors.Wrap(err, "error getting list of loopback devices")
	}
	// Attempt to deactivate the volume group and all of the logical volumes it contains.
	logrus.Debugf("deactivating volume group %q", l.vgname)
	if err = deactivateVolumeGroup(l.vgname); err != nil {
		return errors.Wrapf(err, "error deactivating volume group %q", l.vgname)
	}
	// Attempt to detach any loopback devices which were in the volume group.
	loopbackDevices := make(map[string]*ReportLoopback)
	for i, loopback := range loopbacks.Loopback {
		loopbackDevices[loopback.Name] = &loopbacks.Loopback[i]
	}
	for _, vg := range report.Reports {
		for _, dev := range vg.PVs {
			if dev.VGName != l.vgname {
				logrus.Debugf("checking if device %q is in group %q... no, leaving it be", dev.Name, l.vgname)
				continue
			}
			if _, ok := loopbackDevices[dev.Name]; !ok {
				logrus.Debugf("checking if device %q is a loopback device... no, leaving it be", dev.Name)
				continue
			}
			logrus.Debugf("device %q is a loopback device and in volume group %q, detaching it", dev.Name, l.vgname)
			if err = stopLoopbackDevice(dev.Name); err != nil {
				err = errors.Wrapf(err, "error detaching loopback device %q for volume group %q", dev.Name, l.vgname)
				return err
			}
			logrus.Debugf("detached loopback device %q", dev.Name)
		}
	}
	// Now un-private our top-level.
	if err = mount.Unmount(l.root); err == nil {
		err = errors.Wrapf(err, "error unmounting private mount of %q", l.root)
		return err
	}
	return nil
}

// Return additional locations which have their own lists of layers and images.
func (l *lvmDriver) AdditionalImageStores() []string {
	return nil
}
