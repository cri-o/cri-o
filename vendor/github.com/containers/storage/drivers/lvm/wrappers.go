package lvm

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

var (
	// LVMPath is the path to the "lvm" command.
	LVMPath string
	// LosetupPath is the path to the "losetup" command.
	LosetupPath string
)

func init() {
	if p, err := exec.LookPath("lvm"); err == nil {
		LVMPath = p
	}
	if p, err := exec.LookPath("losetup"); err == nil {
		LosetupPath = p
	}
}

// ReportLoopback represents information about a configured loopback device,
// produced by the "losetup --list" command.
type ReportLoopback struct {
	Name      string `json:"name"`
	SizeLimit int64  `json:"sizelimit,string"`
	Offset    int64  `json:"offset,string"`
	AutoClear int64  `json:"autoclear,string"`
	ReadOnly  int64  `json:"ro,string"`
	File      string `json:"back-file"` // may end with " (deleted)"
	DIO       int64  `json:"dio,string"`
}

// ReportPVCommon represents information that's common about multiple report formats.
type ReportPVCommon struct {
	Name       string `json:"pv_name"`
	Attributes string `json:"pv_attr"`
	Format     string `json:"pv_fmt"`
	Size       int64  `json:"pv_size,string"`
	Free       int64  `json:"pv_free,string"`
}

// ReportPV represents the information about a physical volume that is produced
// by the "lvm pvs" command.
type ReportPV struct {
	ReportPVCommon
	VGName string `json:"vg_name"`
}

// ReportPVFull represents the information about a physical volume that is
// produced by the "lvm fullreport" command.
type ReportPVFull struct {
	ReportPVCommon
	Format        string `json:"pv_fmt"`
	UUID          string `json:"pv_uuid"`
	DeviceSize    int64  `json:"dev_size,string"`
	Major         int64  `json:"pv_major,string"`
	Minor         int64  `json:"pv_minor,string"`
	MDAFree       int64  `json:"pv_mda_free,string"`
	MDASize       int64  `json:"pv_mda_size,string"`
	ExtVersion    int64  `json:"pv_ext_vsn,string"`
	ExtStart      int64  `json:"pe_start,string"`
	Used          int64  `json:"pv_used,string"`
	Allocatable   string `json:"pv_allocatable"`
	Exported      string `json:"pv_exported"`
	Missing       string `json:"pv_missing"`
	ExtCount      int64  `json:"pv_pe_count,string"`
	ExtAllocCount int64  `json:"pv_pe_alloc_count,string"`
	Tags          string `json:"pv_tags"`
	MDACount      int64  `json:"pv_mda_count,string"`
	MDAUsedCount  int64  `json:"pv_mda_used_count,string"`
	BAStart       int64  `json:"pv_ba_start,string"`
	BASize        int64  `json:"pv_ba_size,string"`
	InUse         string `json:"pv_in_use"`
	Duplicate     string `json:"pv_duplicate"`
}

// ReportVGCommon represents information that's common about multiple report formats.
type ReportVGCommon struct {
	Name       string `json:"vg_name"`
	PVCount    int64  `json:"pv_count,string"`
	LVCount    int64  `json:"lv_count,string"`
	SnapCount  int64  `json:"snap_count,string"`
	Attributes string `json:"vg_attr"`
	Size       int64  `json:"vg_size,string"`
	Free       int64  `json:"vg_free,string"`
}

// ReportVG represents the information about a volume group that is produced by
// the "lvm vgs" command.
type ReportVG struct {
	ReportVGCommon
}

// ReportVGFull represents the information about a volume group that is
// produced by the "lvm fullreport" command.
type ReportVGFull struct {
	ReportVGCommon
	Format           string `json:"vg_fmt"`
	UUID             string `json:"vg_uuid"`
	Permissions      string `json:"vg_permissions"`
	Extendable       string `json:"vg_extendable"`
	Exported         string `json:"vg_exported"`
	Partial          string `json:"vg_partial"`
	AllocationPolicy string `json:"vg_allocation_policy"`
	Clustered        string `json:"vg_clustered"`
	SysID            string `json:"vg_sysid"`
	SystemID         string `json:"vg_systemid"`
	LockType         string `json:"vg_locktype"`
	LockArgs         string `json:"vg_lockargs"`
	ExtentSize       int64  `json:"vg_extent_size,string"`
	ExtentCount      int64  `json:"vg_extent_count,string"`
	FreeCount        int64  `json:"vg_free_count,string"`
	MaxLV            int64  `json:"max_lv,string"`
	MaxPV            int64  `json:"max_pv,string"`
	MissingPVCount   int64  `json:"vg_missing_pv_count,string"`
	LVCount          int64  `json:"lv_count,string"`
	SnapCount        int64  `json:"snap_count,string"`
	SequenceNumber   int64  `json:"vg_seqno,string"`
	Tags             string `json:"vg_tags"`
	VGProfile        string `json:"vg_profile"`
	MDACount         int64  `json:"vg_mda_count,string"`
	MDAUsedCount     int64  `json:"vg_mda_used_count,string"`
	MDAFree          int64  `json:"vg_mda_free,string"`
	MDASize          int64  `json:"vg_mda_size,string"`
	MDACopies        string `json:"vg_mda_copies"`
}

// ReportLVCommon represents information that's common about multiple report formats.
type ReportLVCommon struct {
	Name            string `json:"lv_name"`
	Attributes      string `json:"lv_attr"`
	Size            int64  `json:"lv_size,string"`
	Pool            string `json:"pool_lv"`
	Origin          string `json:"origin"`
	DataPercent     string `json:"data_percent"`
	MetadataPercent string `json:"metadata_percent"`
	MovePV          string `json:"move_pv"`
	MirrorLog       string `json:"mirror_log"`
	CopyPercent     string `json:"copy_percent"`
	ConvertLV       string `json:"convert_lv"`
}

// ReportLV represents the information about a logical volume that is produced
// by the "lvm lvs" command.
type ReportLV struct {
	ReportLVCommon
	VGName string `json:"vg_name"`
}

// ReportLVFull represents the information about a logical volume that is
// produced by the "lvm fullreport" command.
type ReportLVFull struct {
	ReportLVCommon
	UUID                string `json:"lv_uuid"`
	FullName            string `json:"lv_full_name"`
	Path                string `json:"lv_path"`
	DMPath              string `json:"lv_dm_path"`
	Parent              string `json:"lv_parent"`
	Layout              string `json:"lv_layout"`
	Role                string `json:"lv_role"`
	InitialImageSync    string `json:"lv_initial_image_sync"`
	ImageSynced         string `json:"lv_image_synced"`
	Merging             string `json:"lv_merging"`
	Converting          string `json:"lv_converting"`
	AllocationPolicy    string `json:"lv_allocation_policy"`
	AllocationLocked    string `json:"lv_allocation_locked"`
	FixedMinor          string `json:"lv_fixed_minor"`
	MergeFailed         string `json:"lv_merge_failed"`
	SnapshotInvalid     string `json:"lv_snapshot_invalid"`
	SkipActivation      string `json:"lv_skip_activation"`
	WhenFull            string `json:"lv_when_full"`
	Active              string `json:"lv_active"`
	ActiveLocally       string `json:"lv_active_locally"`
	ActiveRemotely      string `json:"lv_active_remotely"`
	ActiveExclusively   string `json:"lv_active_exclusively"`
	Major               int64  `json:"lv_major,string"`
	Minor               int64  `json:"lv_minor,string"`
	ReadAhead           string `json:"lv_read_ahead"`
	MetadataSize        string `json:"lv_metadata_size"`
	SegmentCount        int64  `json:"seg_count,string"`
	Origin              string `json:"origin"`
	OriginUUID          string `json:"origin_uuid"`
	OriginSize          string `json:"origin_size"`
	Ancestors           string `json:"lv_ancestors"`
	FullAncestors       string `json:"lv_full_ancestors"`
	Descendants         string `json:"lv_descendants"`
	FullDescendants     string `json:"lv_full_descendants"`
	DataPercent         string `json:"data_percent"`
	SnapPercent         string `json:"snap_percent"`
	MetadataPercent     string `json:"metadata_percent"`
	CopyPercent         string `json:"copy_percent"`
	SyncPercent         string `json:"sync_percent"`
	RAIDMismatchCount   string `json:"raid_mismatch_count"`
	RAIDSyncAction      string `json:"raid_sync_action"`
	RAIDWriteBehind     string `json:"raid_write_behind"`
	RAIDMinRecoveryRate string `json:"raid_min_recovery_rate"`
	RAIDMaxRecoveryRate string `json:"raid_max_recovery_rate"`
	MovePV              string `json:"move_pv"`
	MovePVUUID          string `json:"move_pv_uuid"`
	ConvertLV           string `json:"convert_lv"`
	ConvertLVUUID       string `json:"convert_lv_uuid"`
	MirrorLog           string `json:"mirror_log"`
	MirrorLogUUID       string `json:"mirror_log_uuid"`
	DataLV              string `json:"data_lv"`
	DataLVUUID          string `json:"data_lv_uuid"`
	MetadataLV          string `json:"metadata_lv"`
	MetadataLVUUID      string `json:"metadata_lv_uuid"`
	PoolLV              string `json:"pool_lv"`
	PoolLVUUID          string `json:"pool_lv_uuid"`
	Tags                string `json:"lv_tags"`
	Profile             string `json:"lv_profile"`
	LockArgs            string `json:"lv_lockargs"`
	Time                string `json:"lv_time"`
	TimeRemoved         string `json:"lv_time_removed"`
	Host                string `json:"lv_host"`
	Modules             string `json:"lv_modules"`
	Historical          string `json:"lv_historical"`
	KernelMajor         int64  `json:"lv_kernel_major,string"`
	KernelMinor         int64  `json:"lv_kernel_minor,string"`
	KernelReadAhead     int64  `json:"lv_kernel_read_ahead,string"`
	Permissions         string `json:"lv_permissions"`
	Suspended           string `json:"lv_suspended"`
	LiveTable           string `json:"lv_live_table"`
	InactiveTable       string `json:"lv_inactive_table"`
	DeviceOpen          string `json:"lv_device_open"`
	CacheTotalBlocks    string `json:"cache_total_blocks"`
	CacheUsedBlocks     string `json:"cache_used_blocks"`
	CacheDirtyBlocks    string `json:"cache_dirty_blocks"`
	CacheReadHits       string `json:"cache_read_hits"`
	CacheReadMisses     string `json:"cache_read_misses"`
	CacheWriteHits      string `json:"cache_write_hits"`
	CacheWriteMisses    string `json:"cache_write_misses"`
	KernelCacheSettings string `json:"kernel_cache_settings"`
	KernelCachePolicy   string `json:"kernel_cache_policy"`
	HealthStatus        string `json:"lv_health_status"`
	KernelDiscards      string `json:"kernel_discards"`
	CheckNeeded         string `json:"lv_check_needed"`
}

// ReportEntry represents part of the information about local storage that is
// produced by any of the "lvm vgs", "lvm pvs", or "lvm lvs" command.
type ReportEntry struct {
	PVs []ReportPV `json:"pv"`
	VGs []ReportVG `json:"vg"`
	LVs []ReportLV `json:"lv"`
}

// ReportEntryFull represents the information specific to a local volume group
// that is produced by the "lvm fullreport" command.
type ReportEntryFull struct {
	PVs []ReportPVFull `json:"pv"`
	VGs []ReportVGFull `json:"vg"`
	LVs []ReportLVFull `json:"lv"`
}

// Report represents the information about local storage that is reported by
// any of the "lvm vgs", "lvm pvs", or "lvm lvs" commands, or by the "losetup"
// command.
type Report struct {
	Reports  []ReportEntry    `json:"report,omitempty"`
	Loopback []ReportLoopback `json:"loopdevices,omitempty"`
}

// ReportFull represents the information about local storage that is produced
// by the "lvm fullreport" command.
type ReportFull struct {
	Reports []ReportEntryFull `json:"report,omitempty"`
}

func runWithoutOutput(cmdPath string, args ...string) error {
	logrus.Debugf("running %v", append([]string{cmdPath}, args...))
	cmd := exec.Command(cmdPath, args...)
	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.Errorf("%s", stderr.String())
	}
	return nil
}

func runWithOutput(cmdPath string, args ...string) (string, error) {
	logrus.Debugf("running %v", append([]string{cmdPath}, args...))
	cmd := exec.Command(cmdPath, args...)
	stdout := bytes.Buffer{}
	cmd.Stdout = &stdout
	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), errors.Errorf("%s", stderr.String())
	}
	return stdout.String(), nil
}

// getPhysicalVolumes returns information about known physical volumes or a
// specific physical volume.
func getPhysicalVolumes(pvname string) (Report, error) {
	report := Report{}
	b := []byte{}
	if pvname != "" {
		raw, err := runWithOutput(LVMPath, "pvs", "--reportformat", "json", "--units", "b", "--nosuffix", pvname)
		if err != nil {
			return report, errors.Wrapf(err, "error running \"lvs pvs\" for %q", pvname)
		}
		b = []byte(raw)
	} else {
		raw, err := runWithOutput(LVMPath, "pvs", "--reportformat", "json", "--units", "b", "--nosuffix")
		if err != nil {
			return report, errors.Wrapf(err, "error running \"lvs pvs\"")
		}
		b = []byte(raw)
	}
	err := json.Unmarshal(b, &report)
	if err != nil {
		return Report{}, errors.Wrapf(err, "error decoding output from \"lvs pvs\"")
	}
	return report, nil
}

// getVolumeGroups returns information about the known volume groups, or about
// a specific volume group.
func getVolumeGroups(vgname string) (Report, error) {
	report := Report{}
	b := []byte{}
	if vgname != "" {
		raw, err := runWithOutput(LVMPath, "vgs", "--reportformat", "json", "--units", "b", "--nosuffix", vgname)
		if err != nil {
			return report, errors.Wrapf(err, "error running \"lvs vgs\" for %q", vgname)
		}
		b = []byte(raw)
	} else {
		raw, err := runWithOutput(LVMPath, "vgs", "--all", "--reportformat", "json", "--units", "b", "--nosuffix")
		if err != nil {
			return report, errors.Wrapf(err, "error running \"lvs vgs\"")
		}
		b = []byte(raw)
	}
	err := json.Unmarshal(b, &report)
	if err != nil {
		return Report{}, errors.Wrapf(err, "error decoding output from \"lvs vgs\"")
	}
	return report, nil
}

// parseLoopbackLine parses a line of output from "losetup --list --output NAME,SIZELIMIT,OFFSET,AUTOCLEAR,RO,BACK-FILE"
func parseLoopbackLine(line string) (ReportLoopback, error) {
	var err error
	loopback := ReportLoopback{}
	fields := strings.Fields(line)
	if len(fields) != 6 {
		return loopback, errors.Errorf("error parsing \"losetup --list\" value %q: expected 6 fields, got %d", line, len(fields))
	}
	loopback.Name = fields[0]
	loopback.SizeLimit, err = strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return loopback, errors.Wrapf(err, "error parsing \"losetup --list\" sizelimit value %q", fields[1])
	}
	loopback.Offset, err = strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return loopback, errors.Wrapf(err, "error parsing \"losetup --list\" offset value %q", fields[2])
	}
	loopback.AutoClear, err = strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return loopback, errors.Wrapf(err, "error parsing \"losetup --list\" autoclear value %q", fields[3])
	}
	loopback.ReadOnly, err = strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return loopback, errors.Wrapf(err, "error parsing \"losetup --list\" readonly value %q", fields[4])
	}
	loopback.File = fields[5]
	return loopback, nil
}

// getLoopbacks returns information about active loopback devices.
func getLoopbacks() (Report, error) {
	report := Report{}
	b := []byte{}
	raw, err := runWithOutput(LosetupPath, "--list", "--json")
	if err != nil {
		// --json didn't appear until 2.30rc1
		raw, err = runWithOutput(LosetupPath, "--list", "--output", "NAME,SIZELIMIT,OFFSET,AUTOCLEAR,RO,BACK-FILE")
		if err != nil {
			return report, errors.Wrapf(err, "error running \"losetup --list\"")
		}
		lines := strings.Split(raw, "\n")
		for n, line := range lines[1:] {
			if (n == len(lines[1:])-1) && (line == "") {
				continue
			}

			loopback, err2 := parseLoopbackLine(line)
			if err2 != nil {
				return report, errors.Wrapf(err2, "error parsing \"losetup --list\" value %q", line)
			}
			report.Loopback = append(report.Loopback, loopback)
		}
		return report, nil
	}
	b = []byte(raw)
	err = json.Unmarshal(b, &report)
	if err != nil {
		return Report{}, errors.Wrapf(err, "error decoding output from \"losetup --list\"")
	}
	return report, nil
}

// getVolumeGroupsFull returns detailed information about all known volume
// groups, or about one specific volume group.
func getVolumeGroupsFull(vgname string) (ReportFull, error) {
	report := ReportFull{}
	b := []byte{}
	if vgname != "" {
		raw, err := runWithOutput(LVMPath, "fullreport", "--reportformat", "json", "--units", "b", "--nosuffix", vgname)
		if err != nil {
			return report, errors.Wrapf(err, "error running \"lvm fullreport\" for %q", vgname)
		}
		b = []byte(raw)
	} else {
		raw, err := runWithOutput(LVMPath, "fullreport", "--reportformat", "json", "--units", "b", "--nosuffix")
		if err != nil {
			return report, errors.Wrapf(err, "error running \"lvm fullreport\"")
		}
		b = []byte(raw)
	}
	err := json.Unmarshal(b, &report)
	if err != nil {
		return ReportFull{}, errors.Wrapf(err, "error decoding output from \"lvm fullreport\"")
	}
	return report, nil
}

// getLogicalVolumes returns information about all known logical volumes, about
// the volumes in a specified volume group, or about a specific volume.
func getLogicalVolumes(vgname, volume string) (Report, error) {
	report := Report{}
	b := []byte{}
	if vgname != "" {
		if volume != "" {
			raw, err := runWithOutput(LVMPath, "lvs", "--all", "--reportformat", "json", "--units", "b", "--nosuffix", vgname+"/"+volume)
			if err != nil {
				return report, errors.Wrapf(err, "error running \"lvm lvs\" for %q", vgname+"/"+volume)
			}
			b = []byte(raw)
		} else {
			raw, err := runWithOutput(LVMPath, "lvs", "--all", "--reportformat", "json", "--units", "b", "--nosuffix", vgname)
			if err != nil {
				return report, errors.Wrapf(err, "error running \"lvm lvs\" for %q", vgname)
			}
			b = []byte(raw)
		}
	} else {
		raw, err := runWithOutput(LVMPath, "lvs", "--all", "--reportformat", "json", "--units", "b", "--nosuffix")
		if err != nil {
			return report, errors.Wrapf(err, "error running \"lvm lvs\"")
		}
		b = []byte(raw)
	}
	err := json.Unmarshal(b, &report)
	if err != nil {
		return Report{}, errors.Wrapf(err, "error decoding output from \"lvm lvs\"")
	}
	return report, nil
}

// physicalVolumeIsPresent checks if a physical volume with the specified name
// exists.  Force a rescan of that device for physical volume header data, for
// cases where we've just attached it.
func physicalVolumeIsPresent(pvname string) bool {
	scanned, err := runWithOutput(LVMPath, "pvscan", "--cache", pvname)
	if err != nil {
		logrus.Debugf("lvm pvscan failed: %q", scanned)
		return false
	}
	checked, err := runWithOutput(LVMPath, "pvck", pvname)
	if err != nil {
		logrus.Debugf("lvm pvck failed: %q", checked)
		return false
	}
	return true
}

// volumePathForID determines the device pathname for a volume with the
// specified ID in a particular volume group, or across all volume groups.
func volumePathForID(vgname, id string) (string, error) {
	lvname := volumeNameForID(id)
	report, err := getVolumeGroupsFull(vgname)
	if err != nil {
		return "", errors.WithStack(err)
	}
	for _, entry := range report.Reports {
		if vgname != "" {
			foundVG := false
			for _, vg := range entry.VGs {
				if vg.Name == vgname {
					foundVG = true
					break
				}
			}
			if !foundVG {
				continue
			}
		}
		for _, lv := range entry.LVs {
			if lv.Name == lvname {
				if lv.DMPath != "" {
					_, err = os.Stat(lv.DMPath)
					if err == nil {
						return lv.DMPath, nil
					}
				}
				if lv.Path != "" {
					_, err = os.Stat(lv.Path)
					if err == nil {
						return lv.Path, nil
					}
				}
				return "", errors.Errorf("found LV %q, but no active path for it", vgname+"/"+lv.Name)
			}
		}
	}
	return "", errors.Errorf("no LV named %q found", vgname+"/"+lvname)
}

// readVolumeGroupForPhysicalVolume will determine the name of the volume group
// to which the specified physical volume belongs.
func readVolumeGroupForPhysicalVolume(pvname string) (string, error) {
	report, err := getPhysicalVolumes(pvname)
	if err != nil {
		return "", errors.WithStack(err)
	}
	for _, entry := range report.Reports {
		for _, pv := range entry.PVs {
			if pv.Name == pvname {
				return pv.VGName, nil
			}
		}
	}
	return "", errors.Errorf("no PV named %q found", pvname)
}

// volumeGroupIsPresent checks if a volume group with the specified name exists.
func volumeGroupIsPresent(vgname string) bool {
	scanned, err := runWithOutput(LVMPath, "vgscan", "--cache")
	if err != nil {
		logrus.Debugf("lvm vgscan failed for %q: %q", vgname, scanned)
		return false
	}
	scanned, err = runWithOutput(LVMPath, "vgs", "--reportformat", "json", "--units", "b", "--nosuffix", vgname)
	if err != nil {
		logrus.Debugf("lvm vgs failed for %q: %q", vgname, scanned)
		return false
	}
	return true
}

// getLogicalVolume returns information about the specified logical volume.
func getLogicalVolume(vgname, volume string) (ReportLV, error) {
	report, err := getLogicalVolumes(vgname, volume)
	if err != nil {
		return ReportLV{}, errors.WithStack(err)
	}
	for _, entry := range report.Reports {
		for _, lv := range entry.LVs {
			if lv.Name == volume {
				return lv, nil
			}
		}
	}
	return ReportLV{}, errors.Errorf("no LV named %q found", volume)
}

// logicalVolumeIsPresent checks if a logical volume with the specified name in
// the specified volume group exists.
func logicalVolumeIsPresent(vgname, volume string) bool {
	scanned, err := runWithOutput(LVMPath, "lvscan", "--cache", vgname+"/"+volume)
	if err != nil {
		logrus.Debugf("lvm lvscan failed: %q", scanned)
		return false
	}
	return true
}

// startLoopbackDeviceOnFile mounts the specified file as a loopback device and
// returns the device name which is allocated for it.
func startLoopbackDeviceOnFile(filename string) (string, error) {
	// Take a lock on the file to ensure that two instances of this function won't
	// try to attach it at the same time.
	fd, err := unix.Open(filename, unix.O_RDWR, 0600)
	if err != nil {
		return "", errors.Wrapf(err, "error opening file %q to take a lock", filename)
	}
	defer unix.Close(fd)
	unix.CloseOnExec(fd)
	err = unix.Flock(fd, unix.LOCK_EX)
	if err != nil {
		return "", errors.Wrapf(err, "error locking file %q", filename)
	}
	// --nooverlap didn't appear until 2.30rc1
	output, err := runWithOutput(LosetupPath, "--associated", filename)
	if err != nil {
		return "", errors.Wrapf(err, "error checking if file %q is attached as a loopback device", filename)
	}
	if strings.Index(output, ":") >= 0 {
		output = output[:strings.Index(output, ":")]
	} else {
		output, err = runWithOutput(LosetupPath, "--find", "--show", filename)
		if err != nil {
			return "", errors.Wrapf(err, "error attaching file %q as a loopback device", filename)
		}
	}
	output = strings.TrimRight(output, "\r\n\t ")
	return output, nil
}

// resizeLoopbackDevice tells the kernel that the loopback device's backing file
// may have changed size, so it should check on that.
func resizeLoopbackDevice(device string) error {
	output, err := runWithOutput(LosetupPath, "--set-capacity", device)
	output = strings.TrimRight(output, "\r\n\t ")
	if err != nil {
		return errors.Wrapf(err, "error checking if file under %q has been resized: %q", device, output)
	}
	return nil
}

// stopLoopbackDevice attempts to detach the specified loop device's underlying
// storage file.
func stopLoopbackDevice(device string) error {
	err := runWithoutOutput(LosetupPath, "--detach", device)
	if err != nil {
		return errors.Wrapf(err, "error stopping loopback device %q", device)
	}
	return nil
}

// createPhysicalVolume formats a specified device as a physical volume.
func createPhysicalVolume(device string) error {
	err := runWithoutOutput(LVMPath, "pvcreate", device)
	if err != nil {
		return errors.Wrapf(err, "error running \"lvm pvcreate\" for %q", device)
	}
	return nil
}

// resizePhysicalVolume tells the kernel that the loopback device may be larger
// now, so the volume group that its in will care about that.
func resizePhysicalVolume(device string) error {
	output, err := runWithOutput(LVMPath, "pvresize", device)
	output = strings.TrimRight(output, "\r\n\t ")
	if err != nil {
		return errors.Wrapf(err, "error checking if device %q has been resized: %q", device, output)
	}
	return nil
}

// createPhysicalVolume formats a specified device as a physical volume.
func createVolumeGroup(vgname string, device ...string) error {
	err := runWithoutOutput(LVMPath, append([]string{"vgcreate", vgname}, device...)...)
	if err != nil {
		return errors.Wrapf(err, "error running \"lvm vgcreate\" for %v", device)
	}
	return nil
}

// activateVolumeGroup activates the specified volume group, making all of its
// logical volumes visible.
func activateVolumeGroup(vgname string) error {
	err := runWithoutOutput(LVMPath, "vgchange", "--activate", "y", "--ignoreactivationskip", vgname)
	if err != nil {
		return errors.Wrapf(err, "error running \"lvm vgchange --activate y\" for %q", vgname)
	}
	return nil
}

// deactivateVolumeGroup deactivates the specified volume group, making all of
// its logical volumes invisible.
func deactivateVolumeGroup(vgname string) error {
	err := runWithoutOutput(LVMPath, "vgchange", "--activate", "n", vgname)
	if err != nil {
		return errors.Wrapf(err, "error running \"lvm vgchange --activate n\" for %q", vgname)
	}
	return nil
}

// activateLogicalVolume activates a single logical volume in the specified
// volume group, making it visible.
func activateLogicalVolume(vgname, volume string) error {
	err := runWithoutOutput(LVMPath, "lvchange", "--activate", "y", "--ignoreactivationskip", vgname+"/"+volume)
	if err != nil {
		return errors.Wrapf(err, "error running \"lvm lvchange --activate y\" for %q", vgname+"/"+volume)
	}
	return nil
}

// deactivateLogicalVolume deactivates a single logical volume in the specified
// volume group, making it invisible.
func deactivateLogicalVolume(vgname, volume string) error {
	err := runWithoutOutput(LVMPath, "lvchange", "--activate", "n", vgname+"/"+volume)
	if err != nil {
		return errors.Wrapf(err, "error running \"lvm lvchange --activate n\" for %q", vgname+"/"+volume)
	}
	return nil
}

// read information about the active thin pool
func readPoolInfo(vgname, poolname string) (lvmPoolHistory, error) {
	report, err := getVolumeGroupsFull(vgname)
	if err != nil {
		return lvmPoolHistory{}, errors.Wrapf(err, "error reading information about volume group %q", vgname)
	}
	for _, entry := range report.Reports {
		if vgname != "" {
			foundVG := false
			for _, vg := range entry.VGs {
				if vg.Name == vgname {
					foundVG = true
					break
				}
			}
			if !foundVG {
				continue
			}
		}
		for _, lv := range entry.LVs {
			if lv.Name != poolname {
				continue
			}
			history := lvmPoolHistory{
				VGname:   vgname,
				PoolName: lv.Name,
				PoolUUID: lv.UUID,
			}
			return history, nil
		}
	}
	return lvmPoolHistory{}, errors.Errorf("unable to locate information about pool %q in volume group %q", poolname, vgname)
}
