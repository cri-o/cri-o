//go:build linux

package server

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// dedupProbeSize is the size of the temporary files used to probe whether
// the filesystem supports reflink-based deduplication.
const dedupProbeSize = 4096

// dedupReflinkSupported checks whether the filesystem backing dir supports
// reflink-based deduplication by attempting to deduplicate two temporary,
// identical files via FIDEDUPERANGE. It returns a descriptive error when
// the filesystem does not support reflinks, so callers can distinguish an
// unsupported store from a completed deduplication pass.
func dedupReflinkSupported(dir string) error {
	src, err := os.CreateTemp(dir, ".dedup-probe-src-")
	if err != nil {
		return fmt.Errorf("create dedup probe file: %w", err)
	}
	defer func() {
		_ = src.Close()
		_ = os.Remove(src.Name())
	}()

	dst, err := os.CreateTemp(dir, ".dedup-probe-dst-")
	if err != nil {
		return fmt.Errorf("create dedup probe file: %w", err)
	}
	defer func() {
		_ = dst.Close()
		_ = os.Remove(dst.Name())
	}()

	// Write identical content to both files.
	data := make([]byte, dedupProbeSize)
	if _, err := src.Write(data); err != nil {
		return fmt.Errorf("write dedup probe file: %w", err)
	}

	if _, err := dst.Write(data); err != nil {
		return fmt.Errorf("write dedup probe file: %w", err)
	}

	value := unix.FileDedupeRange{
		Src_offset: 0,
		Src_length: dedupProbeSize,
		Info: []unix.FileDedupeRangeInfo{
			{
				Dest_fd:     int64(dst.Fd()),
				Dest_offset: 0,
			},
		},
	}

	err = unix.IoctlFileDedupeRange(int(src.Fd()), &value)
	if err == nil {
		// The ioctl can succeed while reporting a per-destination failure
		// (e.g. FILE_DEDUPE_RANGE_DIFFERS) via a negative status, or dedup
		// less than the requested range. Treat both as a failed probe.
		info := value.Info[0]
		switch {
		case info.Status < 0:
			err = fmt.Errorf("deduplication of probe files failed: %w", unix.Errno(-info.Status))
		case info.Bytes_deduped != dedupProbeSize:
			err = fmt.Errorf(
				"probe deduplicated %d of %d bytes", info.Bytes_deduped, dedupProbeSize,
			)
		default:
			return nil
		}
	}

	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) ||
		errors.Is(err, unix.ENOTTY) {
		return fmt.Errorf(
			"filesystem backing %q does not support reflinks (FIDEDUPERANGE);"+
				" layer deduplication requires XFS with reflink=1 or Btrfs: %w",
			dir, err,
		)
	}

	// EINVAL can also indicate an invalid or unaligned range, so do not
	// claim with certainty that reflinks are unsupported; point at the
	// likely cause instead.
	if errors.Is(err, unix.EINVAL) {
		return fmt.Errorf(
			"filesystem backing %q rejected the deduplication probe;"+
				" this usually means reflinks are not supported (XFS with"+
				" reflink=1 or Btrfs required): %w",
			dir, err,
		)
	}

	return fmt.Errorf("probe reflink support on %q: %w", dir, err)
}
