package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	"k8s.io/utils/cpuset"
)

type Workloads map[string]*WorkloadConfig

type WorkloadConfig struct {
	// ActivationAnnotation is the pod annotation that activates these workload settings
	ActivationAnnotation string `toml:"activation_annotation"`
	// AnnotationPrefix is the way a pod can override a specific resource for a container.
	// The full annotation must be of the form $annotation_prefix.$resource/$ctrname = $value
	AnnotationPrefix string `toml:"annotation_prefix"`
	// AllowedAnnotations is a slice of experimental annotations that this workload is allowed to process.
	// The currently recognized values are:
	// "io.kubernetes.cri-o.userns-mode" for configuring a user namespace for the pod.
	// "io.kubernetes.cri-o.Devices" for configuring devices for the pod.
	// "io.kubernetes.cri-o.ShmSize" for configuring the size of /dev/shm.
	// "io.kubernetes.cri-o.UnifiedCgroup.$CTR_NAME" for configuring the cgroup v2 unified block for a container.
	// "io.containers.trace-syscall" for tracing syscalls via the OCI seccomp BPF hook.
	AllowedAnnotations []string `toml:"allowed_annotations,omitempty"`
	// DisallowedAnnotations is the slice of experimental annotations that are not allowed for this workload.
	DisallowedAnnotations []string
	// Resources are the names of the resources that can be overridden by annotation.
	// The key of the map is the resource name. The following resources are supported:
	// `cpushares`: configure cpu shares for a given container
	// `cpuset`: configure cpuset for a given container
	// The value of the map is the default value for that resource.
	// If a container is configured to use this workload, and does not specify
	// the annotation with the resource and value, the default value will apply.
	// Default values do not need to be specified.
	Resources *Resources `toml:"resources"`
}

// Resources is a structure for overriding certain resources for the pod.
// This resources structure provides a default value, and can be overridden
// by using the AnnotationPrefix.
type Resources struct {
	// Specifies the number of CPU shares this pod has access to.
	CPUShares uint64 `json:"cpushares,omitempty"`
	// Specifies the cpuset this pod has access to.
	CPUSet string `json:"cpuset,omitempty"`
}

func (w Workloads) Validate() error {
	for workload, config := range w {
		if err := config.Validate(workload); err != nil {
			return err
		}
	}
	return nil
}

func (w *WorkloadConfig) Validate(workloadName string) error {
	if w.ActivationAnnotation == "" {
		return fmt.Errorf("annotation shouldn't be empty for workload %q", workloadName)
	}
	if err := w.ValidateWorkloadAllowedAnnotations(); err != nil {
		return err
	}
	return w.Resources.ValidateDefaults()
}

func (w *WorkloadConfig) ValidateWorkloadAllowedAnnotations() error {
	disallowed, err := validateAllowedAndGenerateDisallowedAnnotations(w.AllowedAnnotations)
	if err != nil {
		return err
	}
	logrus.Debugf(
		"Allowed annotations for workload: %v", w.AllowedAnnotations,
	)
	w.DisallowedAnnotations = disallowed
	return nil
}

func (w Workloads) AllowedAnnotations(toFind map[string]string) []string {
	workload := w.workloadGivenActivationAnnotation(toFind)
	if workload == nil {
		return []string{}
	}
	return workload.AllowedAnnotations
}

// FilterDisallowedAnnotations filters annotations that are not specified in the allowed_annotations map
// for a given handler.
// This function returns an error if the runtime handler can't be found.
// The annotations map is mutated in-place.
func (w Workloads) FilterDisallowedAnnotations(allowed []string, toFilter map[string]string) error {
	disallowed, err := validateAllowedAndGenerateDisallowedAnnotations(allowed)
	if err != nil {
		return err
	}
	logrus.Warnf("Allowed annotations are specified for workload %v", allowed)

	for ann := range toFilter {
		for _, d := range disallowed {
			if strings.HasPrefix(ann, d) {
				delete(toFilter, ann)
			}
		}
	}
	return nil
}

func (w Workloads) MutateSpecGivenAnnotations(ctrName string, specgen *generate.Generator, sboxAnnotations map[string]string) error {
	workload := w.workloadGivenActivationAnnotation(sboxAnnotations)
	if workload == nil {
		return nil
	}
	resources, err := resourcesFromAnnotation(workload.AnnotationPrefix, ctrName, sboxAnnotations, workload.Resources)
	if err != nil {
		return err
	}
	resources.MutateSpec(specgen)

	return nil
}

func (w Workloads) workloadGivenActivationAnnotation(sboxAnnotations map[string]string) *WorkloadConfig {
	for _, wc := range w {
		for annotation := range sboxAnnotations {
			if wc.ActivationAnnotation == annotation {
				return wc
			}
		}
	}
	return nil
}

func resourcesFromAnnotation(prefix, ctrName string, allAnnotations map[string]string, defaultResources *Resources) (*Resources, error) {
	annotationKey := prefix + "/" + ctrName
	value, ok := allAnnotations[annotationKey]
	if !ok {
		return defaultResources, nil
	}

	var resources *Resources
	if err := json.Unmarshal([]byte(value), &resources); err != nil {
		return nil, err
	}
	if resources == nil {
		return nil, nil
	}

	if resources.CPUSet == "" {
		resources.CPUSet = defaultResources.CPUSet
	}
	if resources.CPUShares == 0 {
		resources.CPUShares = defaultResources.CPUShares
	}

	return resources, nil
}

func (r *Resources) ValidateDefaults() error {
	if r == nil {
		return nil
	}
	if r.CPUSet == "" {
		return nil
	}
	_, err := cpuset.Parse(r.CPUSet)
	return err
}

func (r *Resources) MutateSpec(specgen *generate.Generator) {
	if r == nil {
		return
	}
	if r.CPUSet != "" {
		specgen.SetLinuxResourcesCPUCpus(r.CPUSet)
	}
	if r.CPUShares != 0 {
		specgen.SetLinuxResourcesCPUShares(r.CPUShares)
	}
}
