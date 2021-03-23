package workloads

import (
	"strconv"

	"github.com/opencontainers/runtime-tools/generate"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	CPUShareResource = "cpu"
	CPUSetResource   = "cpuset"
)

type Workloads map[string]*WorkloadConfig

type WorkloadConfig struct {
	// Label is the pod label that activates these workload settings
	Label string `toml:"label"`
	// AnnotationPrefix is the way a pod can override a specific resource for a container.
	// The full annotation must be of the form $annotation_prefix.$resource/$ctrname = $value
	AnnotationPrefix string `toml:"annotation_prefix"`
	// Resources are the names of the resources that can be overridden by label.
	// The key of the map is the resource name. The following resources are supported:
	// `cpu`: configure cpu shares for a given container
	// `cpuset`: configure cpuset for a given container
	// The value of the map is the default value for that resource.
	// If a container is configured to use this workload, and does not specify
	// the annotation with the resource and value, the default value will apply.
	// Default values do not need to be specified.
	Resources map[string]string `toml:"resources"`
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
	if w.Label == "" {
		return errors.Errorf("label shouldn't be empty for workload %q", workloadName)
	}
	for resource, defaultValue := range w.Resources {
		validator, ok := resourcesToDefaultValidator[resource]
		if !ok {
			return errors.Errorf("process resource %s for workload %s: resource not supported", resource, workloadName)
		}
		if err := validator(defaultValue); err != nil {
			return errors.Wrapf(err, "process resource %s for workload: default value %s invalid", resource, workloadName, defaultValue)
		}
	}
	return nil
}

var resourcesToDefaultValidator = map[string]func(string) error{
	CPUShareResource: cpuShareDefaultValidator,
	CPUSetResource:   cpusetDefaultValidator,
}

func cpusetDefaultValidator(set string) error {
	if set == "" {
		return nil
	}
	_, err := cpuset.Parse(set)
	return err
}

func cpuShareDefaultValidator(cpuShare string) error {
	if cpuShare == "" {
		return nil
	}
	if _, err := resource.ParseQuantity(cpuShare); err != nil {
		return err
	}
	return nil
}

func (w Workloads) MutateSpecGivenAnnotations(ctrName string, specgen *generate.Generator, sboxLabels, sboxAnnotations map[string]string) error {
	var workload *WorkloadConfig
	for _, wc := range w {
		for label, _ := range sboxLabels {
			if wc.Label == label {
				workload = wc
				break
			}
		}
	}
	if workload == nil {
		return nil
	}
	for resource, defaultValue := range workload.Resources {
		annotationKey := workload.AnnotationPrefix + "." + resource + "/" + ctrName
		value, ok := sboxAnnotations[annotationKey]
		if !ok {
			value = defaultValue
		}
		if value == "" {
			continue
		}

		specMutator, ok := resourcesToSpecMutator[resource]
		if !ok {
			// CRI-O bug
			panic(errors.Errorf("resource %s is not defined", resource))
		}

		if err := specMutator(specgen, value); err != nil {
			return errors.Wrapf(err, "mutating spec given workload %s", workload.Label)
		}
	}
	return nil
}

var resourcesToSpecMutator = map[string]func(*generate.Generator, string) error{
	CPUShareResource: cpuShareSpecMutator,
	CPUSetResource:   cpusetSpecMutator,
}

func cpuShareSpecMutator(specgen *generate.Generator, configuredValue string) error {
	u, err := strconv.ParseUint(configuredValue, 0, 64)
	if err != nil {
		return err
	}
	specgen.SetLinuxResourcesCPUShares(u)
	return nil
}

func cpusetSpecMutator(specgen *generate.Generator, configuredValue string) error {
	specgen.SetLinuxResourcesCPUCpus(configuredValue)
	return nil
}
