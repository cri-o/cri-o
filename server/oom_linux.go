package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/containerd/containerd/sys"
	"github.com/moby/sys/userns"
	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/annotations"
	"github.com/cri-o/cri-o/internal/log"
)

// A process without CAP_SYS_RESOURCE in the initial user namespace can lower
// its OOM score adjustment only down to its oom_score_adj_min, the value which
// a privileged process set last, for example systemd for the unit of CRI-O.
// Processes inherit the minimum, so in a user namespace the OCI runtime fails
// to create a container with a lower value, like the one of the infra
// container or the one of the kubelet for Guaranteed pods.
//
// restrictOOMScoreAdjInUserNS raises these values to the minimum, which keeps
// the containers as protected as allowed. The kernel does not expose the
// minimum, so CRI-O probes it once on its own OOM score adjustment, see
// probeMinOOMScoreAdj. All values below the minimum become equal to it, and if
// the minimum is above the values of other containers, like the ones of
// Burstable pods, the raised containers become OOM victims before them.
//
// With _CRIO_ROOTLESS, makeOCIConfigurationRootless removes the OOM score
// adjustment of all containers instead, so restrictOOMScoreAdjInUserNS does
// nothing then.

const ownOOMScoreAdjPath = "/proc/self/oom_score_adj"

// minOOMScoreAdj returns the lowest OOM score adjustment which CRI-O and the
// processes it starts can set. The minimum only changes if a privileged
// process sets the OOM score adjustment of CRI-O, so it gets probed once.
var minOOMScoreAdj = sync.OnceValues(func() (int, error) {
	return probeMinOOMScoreAdj(
		func() (int, error) { return sys.GetOOMScoreAdj(os.Getpid()) },
		writeOwnOOMScoreAdj,
	)
})

// restrictOOMScoreAdjInUserNS raises the OOM score adjustment of the spec to
// the lowest one which CRI-O can set, if CRI-O runs in a user namespace.
func restrictOOMScoreAdjInUserNS(ctx context.Context, g *generate.Generator) {
	if os.Getenv(rootlessEnvName) != "" || !userns.RunningInUserNS() {
		return
	}

	restrictOOMScoreAdj(ctx, g, minOOMScoreAdj)
}

// restrictOOMScoreAdj raises the OOM score adjustment of the spec to the value
// which lowest returns, if it is lower. The requested value stays in the
// RequestedOOMScoreAdj annotation, which the container status reports.
func restrictOOMScoreAdj(
	ctx context.Context,
	g *generate.Generator,
	lowest func() (int, error),
) {
	if g.Config == nil || g.Config.Process == nil || g.Config.Process.OOMScoreAdj == nil {
		return
	}

	minAdj, err := lowest()
	if err != nil {
		log.Warnf(ctx, "Unable to restrict the OOM score adjustment: %v", err)

		return
	}

	adj := *g.Config.Process.OOMScoreAdj
	if adj >= minAdj {
		return
	}

	log.Debugf(
		ctx,
		"Raising OOM score adjustment from %d to %d, the lowest one allowed in the user namespace",
		adj,
		minAdj,
	)
	g.AddAnnotation(annotations.RequestedOOMScoreAdj, strconv.Itoa(adj))
	g.SetProcessOOMScoreAdj(minAdj)
}

// probeMinOOMScoreAdj returns the lowest OOM score adjustment which write
// accepts, by bisection between the lowest possible value and the current
// one, which is always accepted. Every value from the minimum on gets
// accepted, a failed write counts as below the minimum. The current value
// gets restored afterwards, which is always allowed. Meanwhile, processes
// which CRI-O starts may inherit a lower, but still allowed, value.
func probeMinOOMScoreAdj(
	read func() (int, error),
	write func(int) error,
) (lowest int, retErr error) {
	current, err := read()
	if err != nil {
		return 0, fmt.Errorf("read OOM score adjustment: %w", err)
	}

	low, high := sys.OOMScoreAdjMin, current
	if low >= high {
		return current, nil
	}

	defer func() {
		if err := write(current); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf(
				"restore OOM score adjustment %d: %w", current, err,
			))
		}
	}()

	for low < high {
		mid := low + (high-low)/2
		if write(mid) == nil {
			high = mid
		} else {
			low = mid + 1
		}
	}

	return high, nil
}

// writeOwnOOMScoreAdj sets the OOM score adjustment of CRI-O. SetOOMScore of
// containerd ignores permission errors in a user namespace, which
// probeMinOOMScoreAdj relies on.
func writeOwnOOMScoreAdj(adj int) error {
	f, err := os.OpenFile(ownOOMScoreAdjPath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}

	_, err = f.WriteString(strconv.Itoa(adj))

	return errors.Join(err, f.Close())
}
