/*
Copyright 2016 The Kubernetes Authors.

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

package options

import (
	"fmt"
	"net"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/admission"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	utilflag "k8s.io/apiserver/pkg/util/flag"

	// add the generic feature gates
	_ "k8s.io/apiserver/pkg/features"

	"github.com/spf13/pflag"
)

// ServerRunOptions contains the options while running a generic api server.
type ServerRunOptions struct {
	AdmissionControl           string
	AdmissionControlConfigFile string
	AdvertiseAddress           net.IP

	CorsAllowedOriginList []string
	// To enable protobuf as storage format, it is enough
	// to set it to "application/vnd.kubernetes.protobuf".
	DefaultStorageMediaType     string
	DeleteCollectionWorkers     int
	AuditLogPath                string
	AuditLogMaxAge              int
	AuditLogMaxBackups          int
	AuditLogMaxSize             int
	EnableGarbageCollection     bool
	EnableProfiling             bool
	EnableContentionProfiling   bool
	EnableSwaggerUI             bool
	EnableWatchCache            bool
	ExternalHost                string
	MaxRequestsInFlight         int
	MaxMutatingRequestsInFlight int
	MinRequestTimeout           int
	RuntimeConfig               utilflag.ConfigurationMap
	TargetRAMMB                 int
	WatchCacheSizes             []string
}

func NewServerRunOptions() *ServerRunOptions {
	return &ServerRunOptions{
		AdmissionControl:            "AlwaysAdmit",
		DefaultStorageMediaType:     "application/json",
		DeleteCollectionWorkers:     1,
		EnableGarbageCollection:     true,
		EnableProfiling:             true,
		EnableContentionProfiling:   false,
		EnableWatchCache:            true,
		MaxRequestsInFlight:         400,
		MaxMutatingRequestsInFlight: 200,
		MinRequestTimeout:           1800,
		RuntimeConfig:               make(utilflag.ConfigurationMap),
	}
}

// DefaultAdvertiseAddress sets the field AdvertiseAddress if
// unset. The field will be set based on the SecureServingOptions. If
// the SecureServingOptions is not present, DefaultExternalAddress
// will fall back to the insecure ServingOptions.
func (s *ServerRunOptions) DefaultAdvertiseAddress(secure *SecureServingOptions, insecure *ServingOptions) error {
	if s.AdvertiseAddress == nil || s.AdvertiseAddress.IsUnspecified() {
		switch {
		case secure != nil:
			hostIP, err := secure.ServingOptions.DefaultExternalAddress()
			if err != nil {
				return fmt.Errorf("Unable to find suitable network address.error='%v'. "+
					"Try to set the AdvertiseAddress directly or provide a valid BindAddress to fix this.", err)
			}
			s.AdvertiseAddress = hostIP

		case insecure != nil:
			hostIP, err := insecure.DefaultExternalAddress()
			if err != nil {
				return fmt.Errorf("Unable to find suitable network address.error='%v'. "+
					"Try to set the AdvertiseAddress directly or provide a valid BindAddress to fix this.", err)
			}
			s.AdvertiseAddress = hostIP
		}
	}

	return nil
}

// AddFlags adds flags for a specific APIServer to the specified FlagSet
func (s *ServerRunOptions) AddUniversalFlags(fs *pflag.FlagSet) {
	// Note: the weird ""+ in below lines seems to be the only way to get gofmt to
	// arrange these text blocks sensibly. Grrr.

	fs.StringVar(&s.AdmissionControl, "admission-control", s.AdmissionControl, ""+
		"Ordered list of plug-ins to do admission control of resources into cluster. "+
		"Comma-delimited list of: "+strings.Join(admission.GetPlugins(), ", ")+".")

	fs.StringVar(&s.AdmissionControlConfigFile, "admission-control-config-file", s.AdmissionControlConfigFile,
		"File with admission control configuration.")

	fs.IPVar(&s.AdvertiseAddress, "advertise-address", s.AdvertiseAddress, ""+
		"The IP address on which to advertise the apiserver to members of the cluster. This "+
		"address must be reachable by the rest of the cluster. If blank, the --bind-address "+
		"will be used. If --bind-address is unspecified, the host's default interface will "+
		"be used.")

	fs.StringSliceVar(&s.CorsAllowedOriginList, "cors-allowed-origins", s.CorsAllowedOriginList, ""+
		"List of allowed origins for CORS, comma separated.  An allowed origin can be a regular "+
		"expression to support subdomain matching. If this list is empty CORS will not be enabled.")

	fs.StringVar(&s.DefaultStorageMediaType, "storage-media-type", s.DefaultStorageMediaType, ""+
		"The media type to use to store objects in storage. Defaults to application/json. "+
		"Some resources may only support a specific media type and will ignore this setting.")

	fs.IntVar(&s.DeleteCollectionWorkers, "delete-collection-workers", s.DeleteCollectionWorkers,
		"Number of workers spawned for DeleteCollection call. These are used to speed up namespace cleanup.")

	fs.StringVar(&s.AuditLogPath, "audit-log-path", s.AuditLogPath,
		"If set, all requests coming to the apiserver will be logged to this file.")
	fs.IntVar(&s.AuditLogMaxAge, "audit-log-maxage", s.AuditLogMaxBackups,
		"The maximum number of days to retain old audit log files based on the timestamp encoded in their filename.")
	fs.IntVar(&s.AuditLogMaxBackups, "audit-log-maxbackup", s.AuditLogMaxBackups,
		"The maximum number of old audit log files to retain.")
	fs.IntVar(&s.AuditLogMaxSize, "audit-log-maxsize", s.AuditLogMaxSize,
		"The maximum size in megabytes of the audit log file before it gets rotated. Defaults to 100MB.")

	fs.BoolVar(&s.EnableGarbageCollection, "enable-garbage-collector", s.EnableGarbageCollection, ""+
		"Enables the generic garbage collector. MUST be synced with the corresponding flag "+
		"of the kube-controller-manager.")

	fs.BoolVar(&s.EnableProfiling, "profiling", s.EnableProfiling,
		"Enable profiling via web interface host:port/debug/pprof/")
	fs.BoolVar(&s.EnableContentionProfiling, "contention-profiling", s.EnableContentionProfiling,
		"Enable contention profiling. Requires --profiling to be set to work.")

	fs.BoolVar(&s.EnableSwaggerUI, "enable-swagger-ui", s.EnableSwaggerUI,
		"Enables swagger ui on the apiserver at /swagger-ui")

	// TODO: enable cache in integration tests.
	fs.BoolVar(&s.EnableWatchCache, "watch-cache", s.EnableWatchCache,
		"Enable watch caching in the apiserver")

	fs.IntVar(&s.TargetRAMMB, "target-ram-mb", s.TargetRAMMB,
		"Memory limit for apiserver in MB (used to configure sizes of caches, etc.)")

	fs.StringVar(&s.ExternalHost, "external-hostname", s.ExternalHost,
		"The hostname to use when generating externalized URLs for this master (e.g. Swagger API Docs).")

	// TODO: remove post-1.6
	fs.String("long-running-request-regexp", "", ""+
		"A regular expression matching long running requests which should "+
		"be excluded from maximum inflight request handling.")
	fs.MarkDeprecated("long-running-request-regexp", "regular expression matching of long-running requests is no longer supported")

	deprecatedMasterServiceNamespace := metav1.NamespaceDefault
	fs.StringVar(&deprecatedMasterServiceNamespace, "master-service-namespace", deprecatedMasterServiceNamespace, ""+
		"DEPRECATED: the namespace from which the kubernetes master services should be injected into pods.")

	fs.IntVar(&s.MaxRequestsInFlight, "max-requests-inflight", s.MaxRequestsInFlight, ""+
		"The maximum number of non-mutating requests in flight at a given time. When the server exceeds this, "+
		"it rejects requests. Zero for no limit.")

	fs.IntVar(&s.MaxMutatingRequestsInFlight, "max-mutating-requests-inflight", s.MaxMutatingRequestsInFlight, ""+
		"The maximum number of mutating requests in flight at a given time. When the server exceeds this, "+
		"it rejects requests. Zero for no limit.")

	fs.IntVar(&s.MinRequestTimeout, "min-request-timeout", s.MinRequestTimeout, ""+
		"An optional field indicating the minimum number of seconds a handler must keep "+
		"a request open before timing it out. Currently only honored by the watch request "+
		"handler, which picks a randomized value above this number as the connection timeout, "+
		"to spread out load.")

	fs.Var(&s.RuntimeConfig, "runtime-config", ""+
		"A set of key=value pairs that describe runtime configuration that may be passed "+
		"to apiserver. apis/<groupVersion> key can be used to turn on/off specific api versions. "+
		"apis/<groupVersion>/<resource> can be used to turn on/off specific resources. api/all and "+
		"api/legacy are special keys to control all and legacy api versions respectively.")

	fs.StringSliceVar(&s.WatchCacheSizes, "watch-cache-sizes", s.WatchCacheSizes, ""+
		"List of watch cache sizes for every resource (pods, nodes, etc.), comma separated. "+
		"The individual override format: resource#size, where size is a number. It takes effect "+
		"when watch-cache is enabled.")

	utilfeature.DefaultFeatureGate.AddFlag(fs)
}
