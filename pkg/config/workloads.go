package config

import (
	"encoding/json"

	"github.com/opencontainers/runtime-tools/generate"
	"github.com/pkg/errors"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

type Resources struct {
	CPUShares uint64 `json:"cpushares,omitempty"`
	CPUSet    string `json:"cpuset,omitempty"`
}

type Workloads map[string]*WorkloadConfig

type WorkloadConfig struct {
	// ActivationAnnotation is the pod annotation that activates these workload settings
	ActivationAnnotation string `toml:"activation_annotation"`
	// AnnotationPrefix is the way a pod can override a specific resource for a container.
	// The full annotation must be of the form $annotation_prefix.$resource/$ctrname = $value
	AnnotationPrefix string `toml:"annotation_prefix"`
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
		return errors.Errorf("annotation shouldn't be empty for workload %q", workloadName)
	}
	return w.Resources.ValidateDefaults()
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

func resourcesFromAnnotation(prefix, ctrName string, annotations map[string]string, defaultResources *Resources) (*Resources, error) {
	annotationKey := prefix + "/" + ctrName
	value, ok := annotations[annotationKey]
	if !ok {
		return defaultResources, nil
	}

	var resources *Resources
	if err := json.Unmarshal([]byte(value), &resources); err != nil {
		return nil, err
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
