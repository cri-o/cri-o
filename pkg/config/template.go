package config

import (
	"io"
	"reflect"
	"strings"
	"text/template"
)

// WriteTemplate write the configuration template to the provided writer
func (c *Config) WriteTemplate(displayAllConfig bool, w io.Writer) error {
	const templateName = "config"
	tpl, err := template.New(templateName).Parse(assembleTemplateString(displayAllConfig, c))
	if err != nil {
		return err
	}
	return tpl.ExecuteTemplate(w, templateName, c)
}

func assembleTemplateString(displayAllConfig bool, c *Config) string {
	crioTemplateConfig, err := initCrioTemplateConfig(c)
	if err != nil {
		return ""
	}

	templateString := ""

	// [crio] configuration
	templateString += crioTemplateString(crioRootConfig, "", displayAllConfig, crioTemplateConfig)

	// [crio.api] configuration
	templateString += crioTemplateString(crioAPIConfig, templateStringCrioAPI, displayAllConfig, crioTemplateConfig)

	// [crio.runtime] configuration
	templateString += crioTemplateString(crioRuntimeConfig, templateStringCrioRuntime, displayAllConfig, crioTemplateConfig)

	// [crio.image] configuration
	templateString += crioTemplateString(crioImageConfig, templateStringCrioImage, displayAllConfig, crioTemplateConfig)

	// [crio.network] configuration
	templateString += crioTemplateString(crioNetworkConfig, templateStringCrioNetwork, displayAllConfig, crioTemplateConfig)

	// [crio.metrics] configuration
	templateString += crioTemplateString(crioMetricsConfig, templateStringCrioMetrics, displayAllConfig, crioTemplateConfig)

	// [crio.tracing] configuration
	templateString += crioTemplateString(crioTracingConfig, templateStringCrioTracing, displayAllConfig, crioTemplateConfig)

	// [crio.stats] configuration
	templateString += crioTemplateString(crioStatsConfig, templateStringCrioStats, displayAllConfig, crioTemplateConfig)

	if templateString != "" {
		templateString = templateStringPrefix + templateStringCrio + templateString
	}

	return templateString
}

func crioTemplateString(group templateGroup, prefix string, displayAll bool, crioTemplateConfig []*templateConfigValue) string {
	templateString := ""

	for _, configItem := range crioTemplateConfig {
		if group == configItem.group {
			if !configItem.isDefaultValue || displayAll {
				templateString += strings.ReplaceAll(configItem.templateString, "{{ $.Comment }}", "")
			} else {
				templateString += configItem.templateString
			}
		}
	}

	if templateString != "" {
		templateString = prefix + templateString
	}

	return templateString
}

type templateGroup int32

const (
	crioRootConfig templateGroup = iota + 1
	crioAPIConfig
	crioRuntimeConfig
	crioImageConfig
	crioNetworkConfig
	crioMetricsConfig
	crioTracingConfig
	crioStatsConfig
)

type templateConfigValue struct {
	templateString string
	group          templateGroup
	isDefaultValue bool
}

func initCrioTemplateConfig(c *Config) ([]*templateConfigValue, error) {
	dc, err := DefaultConfig()
	if err != nil {
		return nil, err
	}

	crioTemplateConfig := []*templateConfigValue{
		{
			templateString: templateStringCrioRoot,
			group:          crioRootConfig,
			isDefaultValue: simpleEqual(dc.Root, c.Root),
		},
		{
			templateString: templateStringCrioRunroot,
			group:          crioRootConfig,
			isDefaultValue: simpleEqual(dc.RunRoot, c.RunRoot),
		},
		{
			templateString: templateStringCrioStorageDriver,
			group:          crioRootConfig,
			isDefaultValue: simpleEqual(dc.Storage, c.Storage),
		},
		{
			templateString: templateStringCrioStorageOption,
			group:          crioRootConfig,
			isDefaultValue: stringSliceEqual(dc.StorageOptions, c.StorageOptions),
		},
		{
			templateString: templateStringCrioLogDir,
			group:          crioRootConfig,
			isDefaultValue: simpleEqual(dc.LogDir, c.LogDir),
		},
		{
			templateString: templateStringCrioVersionFile,
			group:          crioRootConfig,
			isDefaultValue: simpleEqual(dc.VersionFile, c.VersionFile),
		},
		{
			templateString: templateStringCrioVersionFilePersist,
			group:          crioRootConfig,
			isDefaultValue: simpleEqual(dc.VersionFilePersist, c.VersionFilePersist),
		},
		{
			templateString: templateStringCrioInternalWipe,
			group:          crioRootConfig,
			isDefaultValue: simpleEqual(dc.InternalWipe, c.InternalWipe),
		},
		{
			templateString: templateStringCrioCleanShutdownFile,
			group:          crioRootConfig,
			isDefaultValue: simpleEqual(dc.CleanShutdownFile, c.CleanShutdownFile),
		},
		{
			templateString: templateStringCrioAPIListen,
			group:          crioAPIConfig,
			isDefaultValue: simpleEqual(dc.Listen, c.Listen),
		},
		{
			templateString: templateStringCrioAPIStreamAddress,
			group:          crioAPIConfig,
			isDefaultValue: simpleEqual(dc.StreamAddress, c.StreamAddress),
		},
		{
			templateString: templateStringCrioAPIStreamPort,
			group:          crioAPIConfig,
			isDefaultValue: simpleEqual(dc.StreamPort, c.StreamPort),
		},
		{
			templateString: templateStringCrioAPIStreamEnableTLS,
			group:          crioAPIConfig,
			isDefaultValue: simpleEqual(dc.StreamEnableTLS, c.StreamEnableTLS),
		},
		{
			templateString: templateStringCrioAPIStreamIdleTimeout,
			group:          crioAPIConfig,
			isDefaultValue: simpleEqual(dc.StreamIdleTimeout, c.StreamIdleTimeout),
		},
		{
			templateString: templateStringCrioAPIStreamTLSCert,
			group:          crioAPIConfig,
			isDefaultValue: simpleEqual(dc.StreamTLSCert, c.StreamTLSCert),
		},
		{
			templateString: templateStringCrioAPIStreamTLSKey,
			group:          crioAPIConfig,
			isDefaultValue: simpleEqual(dc.StreamTLSKey, c.StreamTLSKey),
		},
		{
			templateString: templateStringCrioAPIStreamTLSCa,
			group:          crioAPIConfig,
			isDefaultValue: simpleEqual(dc.StreamTLSCA, c.StreamTLSCA),
		},
		{
			templateString: templateStringCrioAPIGrpcMaxSendMsgSize,
			group:          crioAPIConfig,
			isDefaultValue: simpleEqual(dc.GRPCMaxSendMsgSize, c.GRPCMaxSendMsgSize),
		},
		{
			templateString: templateStringCrioAPIGrpcMaxRecvMsgSize,
			group:          crioAPIConfig,
			isDefaultValue: simpleEqual(dc.GRPCMaxRecvMsgSize, c.GRPCMaxRecvMsgSize),
		},
		{
			templateString: templateStringCrioRuntimeDefaultUlimits,
			group:          crioRuntimeConfig,
			isDefaultValue: stringSliceEqual(dc.DefaultUlimits, c.DefaultUlimits),
		},
		{
			templateString: templateStringCrioRuntimeNoPivot,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.NoPivot, c.NoPivot),
		},
		{
			templateString: templateStringCrioRuntimeDecryptionKeysPath,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.DecryptionKeysPath, c.DecryptionKeysPath),
		},
		{
			templateString: templateStringCrioRuntimeConmon,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.Conmon, c.Conmon),
		},
		{
			templateString: templateStringCrioRuntimeConmonCgroup,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.ConmonCgroup, c.ConmonCgroup),
		},
		{
			templateString: templateStringCrioRuntimeConmonEnv,
			group:          crioRuntimeConfig,
			isDefaultValue: stringSliceEqual(dc.ConmonEnv, c.ConmonEnv),
		},
		{
			templateString: templateStringCrioRuntimeDefaultEnv,
			group:          crioRuntimeConfig,
			isDefaultValue: stringSliceEqual(dc.DefaultEnv, c.DefaultEnv),
		},
		{
			templateString: templateStringCrioRuntimeSelinux,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.SELinux, c.SELinux),
		},
		{
			templateString: templateStringCrioRuntimeSeccompProfile,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.SeccompProfile, c.SeccompProfile),
		},
		{
			templateString: templateStringCrioRuntimeSeccompUseDefaultWhenEmpty,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.SeccompUseDefaultWhenEmpty, c.SeccompUseDefaultWhenEmpty),
		},
		{
			templateString: templateStringCrioRuntimeApparmorProfile,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.ApparmorProfile, c.ApparmorProfile),
		},
		{
			templateString: templateStringCrioRuntimeBlockIOConfigFile,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.BlockIOConfigFile, c.BlockIOConfigFile),
		},
		{
			templateString: templateStringCrioRuntimeIrqBalanceConfigFile,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.IrqBalanceConfigFile, c.IrqBalanceConfigFile),
		},
		{
			templateString: templateStringCrioRuntimeRdtConfigFile,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.RdtConfigFile, c.RdtConfigFile),
		},
		{
			templateString: templateStringCrioRuntimeCgroupManager,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.CgroupManagerName, c.CgroupManagerName),
		},
		{
			templateString: templateStringCrioRuntimeSeparatePullCgroup,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.SeparatePullCgroup, c.SeparatePullCgroup),
		},
		{
			templateString: templateStringCrioRuntimeDefaultCapabilities,
			group:          crioRuntimeConfig,
			isDefaultValue: stringSliceEqual(dc.DefaultCapabilities, c.DefaultCapabilities),
		},
		{
			templateString: templateStringCrioRuntimeAddInheritableCapabilities,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.AddInheritableCapabilities, c.AddInheritableCapabilities),
		},
		{
			templateString: templateStringCrioRuntimeDefaultSysctls,
			group:          crioRuntimeConfig,
			isDefaultValue: stringSliceEqual(dc.DefaultSysctls, c.DefaultSysctls),
		},
		{
			templateString: templateStringCrioRuntimeAllowedDevices,
			group:          crioRuntimeConfig,
			isDefaultValue: stringSliceEqual(dc.AllowedDevices, c.AllowedDevices),
		},
		{
			templateString: templateStringCrioRuntimeAdditionalDevices,
			group:          crioRuntimeConfig,
			isDefaultValue: stringSliceEqual(dc.AdditionalDevices, c.AdditionalDevices),
		},
		{
			templateString: templateStringCrioRuntimeCDISpecDirs,
			group:          crioRuntimeConfig,
			isDefaultValue: stringSliceEqual(dc.CDISpecDirs, c.CDISpecDirs),
		},
		{
			templateString: templateStringCrioRuntimeDeviceOwnershipFromSecurityContext,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.DeviceOwnershipFromSecurityContext, c.DeviceOwnershipFromSecurityContext),
		},
		{
			templateString: templateStringCrioRuntimeHooksDir,
			group:          crioRuntimeConfig,
			isDefaultValue: stringSliceEqual(dc.HooksDir, c.HooksDir),
		},
		{
			templateString: templateStringCrioRuntimeDefaultMountsFile,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.DefaultMountsFile, c.DefaultMountsFile),
		},
		{
			templateString: templateStringCrioRuntimePidsLimit,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.PidsLimit, c.PidsLimit),
		},
		{
			templateString: templateStringCrioRuntimeLogSizeMax,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.LogSizeMax, c.LogSizeMax),
		},
		{
			templateString: templateStringCrioRuntimeLogToJournald,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.LogToJournald, c.LogToJournald),
		},
		{
			templateString: templateStringCrioRuntimeContainerExitsDir,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.ContainerExitsDir, c.ContainerExitsDir),
		},
		{
			templateString: templateStringCrioRuntimeContainerAttachSocketDir,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.ContainerAttachSocketDir, c.ContainerAttachSocketDir),
		},
		{
			templateString: templateStringCrioRuntimeBindMountPrefix,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.BindMountPrefix, c.BindMountPrefix),
		},
		{
			templateString: templateStringCrioRuntimeReadOnly,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.ReadOnly, c.ReadOnly),
		},
		{
			templateString: templateStringCrioRuntimeLogLevel,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.LogLevel, c.LogLevel),
		},
		{
			templateString: templateStringCrioRuntimeLogFilter,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.LogFilter, c.LogFilter),
		},
		{
			templateString: templateStringCrioRuntimeUIDMappings,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.UIDMappings, c.UIDMappings),
		},
		{
			templateString: templateStringCrioRuntimeGIDMappings,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.GIDMappings, c.GIDMappings),
		},
		{
			templateString: templateStringCrioRuntimeMinimumMappableUID,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.MinimumMappableUID, c.MinimumMappableUID),
		},
		{
			templateString: templateStringCrioRuntimeMinimumMappableGID,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.MinimumMappableGID, c.MinimumMappableGID),
		},
		{
			templateString: templateStringCrioRuntimeCtrStopTimeout,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.CtrStopTimeout, c.CtrStopTimeout),
		},
		{
			templateString: templateStringCrioRuntimeDropInfraCtr,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.DropInfraCtr, c.DropInfraCtr),
		},
		{
			templateString: templateStringCrioRuntimeInfraCtrCpuset,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.InfraCtrCPUSet, c.InfraCtrCPUSet),
		},
		{
			templateString: templateStringCrioRuntimeNamespacesDir,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.NamespacesDir, c.NamespacesDir),
		},
		{
			templateString: templateStringCrioRuntimePinnsPath,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.PinnsPath, c.PinnsPath),
		},
		{
			templateString: templateStringCrioRuntimeEnableCriuSupport,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.EnableCriuSupport, c.EnableCriuSupport),
		},
		{
			templateString: templateStringCrioRuntimeDefaultRuntime,
			group:          crioRuntimeConfig,
			isDefaultValue: simpleEqual(dc.DefaultRuntime, c.DefaultRuntime),
		},
		{
			templateString: templateStringCrioRuntimeAbsentMountSourcesToReject,
			group:          crioRuntimeConfig,
			isDefaultValue: stringSliceEqual(dc.AbsentMountSourcesToReject, c.AbsentMountSourcesToReject),
		},
		{
			templateString: templateStringCrioRuntimeRuntimesRuntimeHandler,
			group:          crioRuntimeConfig,
			isDefaultValue: RuntimesEqual(dc.Runtimes, c.Runtimes),
		},
		{
			templateString: templateStringCrioRuntimeWorkloads,
			group:          crioRuntimeConfig,
			isDefaultValue: WorkloadsEqual(dc.Workloads, c.Workloads),
		},
		{
			templateString: templateStringCrioImageDefaultTransport,
			group:          crioImageConfig,
			isDefaultValue: simpleEqual(dc.DefaultTransport, c.DefaultTransport),
		},
		{
			templateString: templateStringCrioImageGlobalAuthFile,
			group:          crioImageConfig,
			isDefaultValue: simpleEqual(dc.GlobalAuthFile, c.GlobalAuthFile),
		},
		{
			templateString: templateStringCrioImagePauseImage,
			group:          crioImageConfig,
			isDefaultValue: simpleEqual(dc.PauseImage, c.PauseImage),
		},
		{
			templateString: templateStringCrioImagePauseImageAuthFile,
			group:          crioImageConfig,
			isDefaultValue: simpleEqual(dc.PauseImageAuthFile, c.PauseImageAuthFile),
		},
		{
			templateString: templateStringCrioImagePauseCommand,
			group:          crioImageConfig,
			isDefaultValue: simpleEqual(dc.PauseCommand, c.PauseCommand),
		},
		{
			templateString: templateStringCrioImageSignaturePolicy,
			group:          crioImageConfig,
			isDefaultValue: simpleEqual(dc.SignaturePolicyPath, c.SignaturePolicyPath),
		},
		{
			templateString: templateStringCrioImageInsecureRegistries,
			group:          crioImageConfig,
			isDefaultValue: stringSliceEqual(dc.InsecureRegistries, c.InsecureRegistries),
		},
		{
			templateString: templateStringCrioImageImageVolumes,
			group:          crioImageConfig,
			isDefaultValue: simpleEqual(dc.ImageVolumes, c.ImageVolumes),
		},
		{
			templateString: templateStringCrioImageBigFilesTemporaryDir,
			group:          crioImageConfig,
			isDefaultValue: simpleEqual(dc.BigFilesTemporaryDir, c.BigFilesTemporaryDir),
		},
		{
			templateString: templateStringCrioNetworkCniDefaultNetwork,
			group:          crioNetworkConfig,
			isDefaultValue: simpleEqual(dc.CNIDefaultNetwork, c.CNIDefaultNetwork),
		},
		{
			templateString: templateStringCrioNetworkNetworkDir,
			group:          crioNetworkConfig,
			isDefaultValue: simpleEqual(dc.NetworkDir, c.NetworkDir),
		},
		{
			templateString: templateStringCrioNetworkPluginDirs,
			group:          crioNetworkConfig,
			isDefaultValue: stringSliceEqual(dc.PluginDirs, c.PluginDirs),
		},
		{
			templateString: templateStringCrioMetricsEnableMetrics,
			group:          crioMetricsConfig,
			isDefaultValue: simpleEqual(dc.EnableMetrics, c.EnableMetrics),
		},
		{
			templateString: templateStringCrioMetricsCollectors,
			group:          crioMetricsConfig,
			isDefaultValue: stringSliceEqual(dc.MetricsCollectors.ToSlice(), c.MetricsCollectors.ToSlice()),
		},
		{
			templateString: templateStringCrioMetricsMetricsPort,
			group:          crioMetricsConfig,
			isDefaultValue: simpleEqual(dc.MetricsPort, c.MetricsPort),
		},
		{
			templateString: templateStringCrioMetricsMetricsSocket,
			group:          crioMetricsConfig,
			isDefaultValue: simpleEqual(dc.MetricsSocket, c.MetricsSocket),
		},
		{
			templateString: templateStringCrioMetricsMetricsCert,
			group:          crioMetricsConfig,
			isDefaultValue: simpleEqual(dc.MetricsCert, c.MetricsCert),
		},
		{
			templateString: templateStringCrioMetricsMetricsKey,
			group:          crioMetricsConfig,
			isDefaultValue: simpleEqual(dc.MetricsKey, c.MetricsKey),
		},
		{
			templateString: templateStringCrioTracingEnableTracing,
			group:          crioTracingConfig,
			isDefaultValue: simpleEqual(dc.EnableTracing, c.EnableTracing),
		},
		{
			templateString: templateStringCrioTracingTracingEndpoint,
			group:          crioTracingConfig,
			isDefaultValue: simpleEqual(dc.TracingEndpoint, c.TracingEndpoint),
		},
		{
			templateString: templateStringCrioTracingTracingSamplingRatePerMillion,
			group:          crioTracingConfig,
			isDefaultValue: simpleEqual(dc.TracingSamplingRatePerMillion, c.TracingSamplingRatePerMillion),
		},
		{
			templateString: templateStringCrioTracingTracingSampleAlways,
			group:          crioTracingConfig,
			isDefaultValue: simpleEqual(dc.TracingSampleAlways, c.TracingSampleAlways),
		},
		{
			templateString: templateStringCrioStatsStatsCollectionPeriod,
			group:          crioStatsConfig,
			isDefaultValue: simpleEqual(dc.StatsCollectionPeriod, c.StatsCollectionPeriod),
		},
	}

	return crioTemplateConfig, nil
}

func simpleEqual(a, b interface{}) bool {
	return a == b
}

func stringSliceEqual(a, b []string) bool {
	if (a == nil) && (b == nil) {
		return true
	}

	if (a == nil) && (len(b) == 0) {
		return true
	}

	if (b == nil) && (len(a) == 0) {
		return true
	}

	if len(a) != len(b) {
		return false
	}

	for i, v := range a {
		if v != b[i] {
			return false
		}
	}

	return true
}

func RuntimesEqual(a, b Runtimes) bool {
	if len(a) != len(b) {
		return false
	}

	for key, valueA := range a {
		valueB, ok := b[key]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(valueA, valueB) {
			return false
		}
	}

	return true
}

func WorkloadsEqual(a, b Workloads) bool {
	if len(a) != len(b) {
		return false
	}

	for key, valueA := range a {
		valueB, ok := b[key]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(valueA, valueB) {
			return false
		}
	}

	return true
}

const templateStringPrefix = `# The CRI-O configuration file specifies all of the available configuration
# options and command-line flags for the crio(8) OCI Kubernetes Container Runtime
# daemon, but in a TOML format that can be more easily modified and versioned.
#
# Please refer to crio.conf(5) for details of all configuration options.

# CRI-O supports partial configuration reload during runtime, which can be
# done by sending SIGHUP to the running process. Currently supported options
# are explicitly mentioned with: 'This option supports live configuration
# reload'.

# CRI-O reads its storage defaults from the containers-storage.conf(5) file
# located at /etc/containers/storage.conf. Modify this storage configuration if
# you want to change the system's defaults. If you want to modify storage just
# for CRI-O, you can change the storage configuration options here.
`

const templateStringCrio = `[crio]

`

const templateStringCrioRoot = `# Path to the "root directory". CRI-O stores all of its data, including
# containers images, in this directory.
{{ $.Comment }}root = "{{ .Root }}"

`

const templateStringCrioRunroot = `# Path to the "run directory". CRI-O stores all of its state in this directory.
{{ $.Comment }}runroot = "{{ .RunRoot }}"

`

const templateStringCrioStorageDriver = `# Storage driver used to manage the storage of images and containers. Please
# refer to containers-storage.conf(5) to see all available storage drivers.
{{ $.Comment }}storage_driver = "{{ .Storage }}"

`

const templateStringCrioStorageOption = `# List to pass options to the storage driver. Please refer to
# containers-storage.conf(5) to see all available storage options.
{{ $.Comment }}storage_option = [
{{ range $opt := .StorageOptions }}{{ $.Comment }}{{ printf "\t%q,\n" $opt }}{{ end }}{{ $.Comment }}]

`

const templateStringCrioLogDir = `# The default log directory where all logs will go unless directly specified by
# the kubelet. The log directory specified must be an absolute directory.
{{ $.Comment }}log_dir = "{{ .LogDir }}"

`

const templateStringCrioVersionFile = `# Location for CRI-O to lay down the temporary version file.
# It is used to check if crio wipe should wipe containers, which should
# always happen on a node reboot
{{ $.Comment }}version_file = "{{ .VersionFile }}"

`

const templateStringCrioVersionFilePersist = `# Location for CRI-O to lay down the persistent version file.
# It is used to check if crio wipe should wipe images, which should
# only happen when CRI-O has been upgraded
{{ $.Comment }}version_file_persist = "{{ .VersionFilePersist }}"

`

const templateStringCrioCleanShutdownFile = `# Location for CRI-O to lay down the clean shutdown file.
# It is used to check whether crio had time to sync before shutting down.
# If not found, crio wipe will clear the storage directory.
{{ $.Comment }}clean_shutdown_file = "{{ .CleanShutdownFile }}"

`

const templateStringCrioInternalWipe = `# InternalWipe is whether CRI-O should wipe containers and images after a reboot when the server starts.
# If set to false, one must use the external command 'crio wipe' to wipe the containers and images in these situations.
{{ $.Comment }}internal_wipe = {{ .InternalWipe }}

`

const templateStringCrioAPI = `# The crio.api table contains settings for the kubelet/gRPC interface.
[crio.api]

`

const templateStringCrioAPIListen = `# Path to AF_LOCAL socket on which CRI-O will listen.
{{ $.Comment }}listen = "{{ .Listen }}"

`

const templateStringCrioAPIStreamAddress = `# IP address on which the stream server will listen.
{{ $.Comment }}stream_address = "{{ .StreamAddress }}"

`

const templateStringCrioAPIStreamPort = `# The port on which the stream server will listen. If the port is set to "0", then
# CRI-O will allocate a random free port number.
{{ $.Comment }}stream_port = "{{ .StreamPort }}"

`

const templateStringCrioAPIStreamEnableTLS = `# Enable encrypted TLS transport of the stream server.
{{ $.Comment }}stream_enable_tls = {{ .StreamEnableTLS }}

`

const templateStringCrioAPIStreamIdleTimeout = `# Length of time until open streams terminate due to lack of activity
{{ $.Comment }}stream_idle_timeout = "{{.StreamIdleTimeout}}"

`

const templateStringCrioAPIStreamTLSCert = `# Path to the x509 certificate file used to serve the encrypted stream. This
# file can change, and CRI-O will automatically pick up the changes within 5
# minutes.
{{ $.Comment }}stream_tls_cert = "{{ .StreamTLSCert }}"

`

const templateStringCrioAPIStreamTLSKey = `# Path to the key file used to serve the encrypted stream. This file can
# change and CRI-O will automatically pick up the changes within 5 minutes.
{{ $.Comment }}stream_tls_key = "{{ .StreamTLSKey }}"

`

const templateStringCrioAPIStreamTLSCa = `# Path to the x509 CA(s) file used to verify and authenticate client
# communication with the encrypted stream. This file can change and CRI-O will
# automatically pick up the changes within 5 minutes.
{{ $.Comment }}stream_tls_ca = "{{ .StreamTLSCA }}"

`

const templateStringCrioAPIGrpcMaxSendMsgSize = `# Maximum grpc send message size in bytes. If not set or <=0, then CRI-O will default to 16 * 1024 * 1024.
{{ $.Comment }}grpc_max_send_msg_size = {{ .GRPCMaxSendMsgSize }}

`

const templateStringCrioAPIGrpcMaxRecvMsgSize = `# Maximum grpc receive message size. If not set or <= 0, then CRI-O will default to 16 * 1024 * 1024.
{{ $.Comment }}grpc_max_recv_msg_size = {{ .GRPCMaxRecvMsgSize }}

`

const templateStringCrioRuntime = `# The crio.runtime table contains settings pertaining to the OCI runtime used
# and options for how to set up and manage the OCI runtime.
[crio.runtime]

`

const templateStringCrioRuntimeDefaultUlimits = `# A list of ulimits to be set in containers by default, specified as
# "<ulimit name>=<soft limit>:<hard limit>", for example:
# "nofile=1024:2048"
# If nothing is set here, settings will be inherited from the CRI-O daemon
{{ $.Comment }}default_ulimits = [
{{ range $ulimit := .DefaultUlimits }}{{ $.Comment }}{{ printf "\t%q,\n" $ulimit }}{{ end }}{{ $.Comment }}]

`

const templateStringCrioRuntimeNoPivot = `# If true, the runtime will not use pivot_root, but instead use MS_MOVE.
{{ $.Comment }}no_pivot = {{ .NoPivot }}

`

const templateStringCrioRuntimeDecryptionKeysPath = `# decryption_keys_path is the path where the keys required for
# image decryption are stored. This option supports live configuration reload.
{{ $.Comment }}decryption_keys_path = "{{ .DecryptionKeysPath }}"

`

const templateStringCrioRuntimeConmon = `# Path to the conmon binary, used for monitoring the OCI runtime.
# Will be searched for using $PATH if empty.
# This option is currently deprecated, and will be replaced with RuntimeHandler.MonitorEnv.
{{ $.Comment }}conmon = "{{ .Conmon }}"

`

const templateStringCrioRuntimeConmonCgroup = `# Cgroup setting for conmon
# This option is currently deprecated, and will be replaced with RuntimeHandler.MonitorCgroup.
{{ $.Comment }}conmon_cgroup = "{{ .ConmonCgroup }}"

`

const templateStringCrioRuntimeConmonEnv = `# Environment variable list for the conmon process, used for passing necessary
# environment variables to conmon or the runtime.
# This option is currently deprecated, and will be replaced with RuntimeHandler.MonitorEnv.
{{ $.Comment }}conmon_env = [
{{ range $env := .ConmonEnv }}{{ $.Comment }}{{ printf "\t%q,\n" $env }}{{ end }}{{ $.Comment }}]

`

const templateStringCrioRuntimeDefaultEnv = `# Additional environment variables to set for all the
# containers. These are overridden if set in the
# container image spec or in the container runtime configuration.
{{ $.Comment }}default_env = [
{{ range $env := .DefaultEnv }}{{ $.Comment }}{{ printf "\t%q,\n" $env }}{{ end }}{{ $.Comment }}]

`

const templateStringCrioRuntimeSelinux = `# If true, SELinux will be used for pod separation on the host.
{{ $.Comment }}selinux = {{ .SELinux }}

`

const templateStringCrioRuntimeSeccompProfile = `# Path to the seccomp.json profile which is used as the default seccomp profile
# for the runtime. If not specified, then the internal default seccomp profile
# will be used. This option supports live configuration reload.
{{ $.Comment }}seccomp_profile = "{{ .SeccompProfile }}"

`

const templateStringCrioRuntimeSeccompUseDefaultWhenEmpty = `# Changes the meaning of an empty seccomp profile. By default
# (and according to CRI spec), an empty profile means unconfined.
# This option tells CRI-O to treat an empty profile as the default profile,
# which might increase security.
{{ $.Comment }}seccomp_use_default_when_empty = {{ .SeccompUseDefaultWhenEmpty }}

`

const templateStringCrioRuntimeApparmorProfile = `# Used to change the name of the default AppArmor profile of CRI-O. The default
# profile name is "crio-default". This profile only takes effect if the user
# does not specify a profile via the Kubernetes Pod's metadata annotation. If
# the profile is set to "unconfined", then this equals to disabling AppArmor.
# This option supports live configuration reload.
{{ $.Comment }}apparmor_profile = "{{ .ApparmorProfile }}"

`

const templateStringCrioRuntimeBlockIOConfigFile = `# Path to the blockio class configuration file for configuring
# the cgroup blockio controller.
{{ $.Comment }}blockio_config_file = "{{ .BlockIOConfigFile }}"

`

const templateStringCrioRuntimeIrqBalanceConfigFile = `# Used to change irqbalance service config file path which is used for configuring
# irqbalance daemon.
{{ $.Comment }}irqbalance_config_file = "{{ .IrqBalanceConfigFile }}"

`

const templateStringCrioRuntimeRdtConfigFile = `# Path to the RDT configuration file for configuring the resctrl pseudo-filesystem.
# This option supports live configuration reload.
{{ $.Comment }}rdt_config_file = "{{ .RdtConfigFile }}"

`

const templateStringCrioRuntimeCgroupManager = `# Cgroup management implementation used for the runtime.
{{ $.Comment }}cgroup_manager = "{{ .CgroupManagerName }}"

`

const templateStringCrioRuntimeSeparatePullCgroup = `# Specify whether the image pull must be performed in a separate cgroup.
{{ $.Comment }}separate_pull_cgroup = "{{ .SeparatePullCgroup }}"

`

const templateStringCrioRuntimeDefaultCapabilities = `# List of default capabilities for containers. If it is empty or commented out,
# only the capabilities defined in the containers json file by the user/kube
# will be added.
{{ $.Comment }}default_capabilities = [
{{ range $capability := .DefaultCapabilities}}{{ $.Comment }}{{ printf "\t%q,\n" $capability}}{{ end }}{{ $.Comment }}]

`

const templateStringCrioRuntimeAddInheritableCapabilities = `# Add capabilities to the inheritable set, as well as the default group of permitted, bounding and effective.
# If capabilities are expected to work for non-root users, this option should be set.
{{ $.Comment }}add_inheritable_capabilities = {{ .AddInheritableCapabilities }}

`

const templateStringCrioRuntimeDefaultSysctls = `# List of default sysctls. If it is empty or commented out, only the sysctls
# defined in the container json file by the user/kube will be added.
{{ $.Comment }}default_sysctls = [
{{ range $sysctl := .DefaultSysctls}}{{ $.Comment }}{{ printf "\t%q,\n" $sysctl}}{{ end }}{{ $.Comment }}]

`

const templateStringCrioRuntimeAllowedDevices = `# List of devices on the host that a
# user can specify with the "io.kubernetes.cri-o.Devices" allowed annotation.
{{ $.Comment }}allowed_devices = [
{{ range $device := .AllowedDevices}}{{ $.Comment }}{{ printf "\t%q,\n" $device}}{{ end }}{{ $.Comment }}]

`

const templateStringCrioRuntimeAdditionalDevices = `# List of additional devices. specified as
# "<device-on-host>:<device-on-container>:<permissions>", for example: "--device=/dev/sdc:/dev/xvdc:rwm".
# If it is empty or commented out, only the devices
# defined in the container json file by the user/kube will be added.
{{ $.Comment }}additional_devices = [
{{ range $device := .AdditionalDevices}}{{ $.Comment }}{{ printf "\t%q,\n" $device}}{{ end }}{{ $.Comment }}]

`

const templateStringCrioRuntimeCDISpecDirs = `# List of directories to scan for CDI Spec files.
{{ $.Comment }}cdi_spec_dirs = [
{{ range $dir := .CDISpecDirs }}{{ $.Comment }}{{ printf "\t%q,\n" $dir}}{{ end }}{{ $.Comment }}]

`

const templateStringCrioRuntimeDeviceOwnershipFromSecurityContext = `# Change the default behavior of setting container devices uid/gid from CRI's
# SecurityContext (RunAsUser/RunAsGroup) instead of taking host's uid/gid.
# Defaults to false.
{{ $.Comment }}device_ownership_from_security_context = {{ .DeviceOwnershipFromSecurityContext }}

`

const templateStringCrioRuntimeHooksDir = `# Path to OCI hooks directories for automatically executed hooks. If one of the
# directories does not exist, then CRI-O will automatically skip them.
{{ $.Comment }}hooks_dir = [
{{ range $hooksDir := .HooksDir }}{{ $.Comment }}{{ printf "\t%q,\n" $hooksDir}}{{ end }}{{ $.Comment }}]

`

const templateStringCrioRuntimeDefaultMountsFile = `# Path to the file specifying the defaults mounts for each container. The
# format of the config is /SRC:/DST, one mount per line. Notice that CRI-O reads
# its default mounts from the following two files:
#
#   1) /etc/containers/mounts.conf (i.e., default_mounts_file): This is the
#      override file, where users can either add in their own default mounts, or
#      override the default mounts shipped with the package.
#
#   2) /usr/share/containers/mounts.conf: This is the default file read for
#      mounts. If you want CRI-O to read from a different, specific mounts file,
#      you can change the default_mounts_file. Note, if this is done, CRI-O will
#      only add mounts it finds in this file.
#
{{ $.Comment }}default_mounts_file = "{{ .DefaultMountsFile }}"

`

const templateStringCrioRuntimePidsLimit = `# Maximum number of processes allowed in a container.
# This option is deprecated. The Kubelet flag '--pod-pids-limit' should be used instead.
{{ $.Comment }}pids_limit = {{ .PidsLimit }}

`

const templateStringCrioRuntimeLogSizeMax = `# Maximum sized allowed for the container log file. Negative numbers indicate
# that no size limit is imposed. If it is positive, it must be >= 8192 to
# match/exceed conmon's read buffer. The file is truncated and re-opened so the
# limit is never exceeded. This option is deprecated. The Kubelet flag '--container-log-max-size' should be used instead.
{{ $.Comment }}log_size_max = {{ .LogSizeMax }}

`

const templateStringCrioRuntimeLogToJournald = `# Whether container output should be logged to journald in addition to the kuberentes log file
{{ $.Comment }}log_to_journald = {{ .LogToJournald }}

`

const templateStringCrioRuntimeContainerExitsDir = `# Path to directory in which container exit files are written to by conmon.
{{ $.Comment }}container_exits_dir = "{{ .ContainerExitsDir }}"

`

const templateStringCrioRuntimeContainerAttachSocketDir = `# Path to directory for container attach sockets.
{{ $.Comment }}container_attach_socket_dir = "{{ .ContainerAttachSocketDir }}"

`

const templateStringCrioRuntimeBindMountPrefix = `# The prefix to use for the source of the bind mounts.
{{ $.Comment }}bind_mount_prefix = ""

`

const templateStringCrioRuntimeReadOnly = `# If set to true, all containers will run in read-only mode.
{{ $.Comment }}read_only = {{ .ReadOnly }}

`

const templateStringCrioRuntimeLogLevel = `# Changes the verbosity of the logs based on the level it is set to. Options
# are fatal, panic, error, warn, info, debug and trace. This option supports
# live configuration reload.
{{ $.Comment }}log_level = "{{ .LogLevel }}"

`

const templateStringCrioRuntimeLogFilter = `# Filter the log messages by the provided regular expression.
# This option supports live configuration reload.
{{ $.Comment }}log_filter = "{{ .LogFilter }}"

`

const templateStringCrioRuntimeUIDMappings = `# The UID mappings for the user namespace of each container. A range is
# specified in the form containerUID:HostUID:Size. Multiple ranges must be
# separated by comma.
{{ $.Comment }}uid_mappings = "{{ .UIDMappings }}"

`

const templateStringCrioRuntimeGIDMappings = `# The GID mappings for the user namespace of each container. A range is
# specified in the form containerGID:HostGID:Size. Multiple ranges must be
# separated by comma.
{{ $.Comment }}gid_mappings = "{{ .GIDMappings }}"

`

const templateStringCrioRuntimeMinimumMappableUID = `# If set, CRI-O will reject any attempt to map host UIDs below this value
# into user namespaces.  A negative value indicates that no minimum is set,
# so specifying mappings will only be allowed for pods that run as UID 0.
{{ $.Comment }}minimum_mappable_uid = {{ .MinimumMappableUID }}

`

const templateStringCrioRuntimeMinimumMappableGID = `# If set, CRI-O will reject any attempt to map host GIDs below this value
# into user namespaces.  A negative value indicates that no minimum is set,
# so specifying mappings will only be allowed for pods that run as UID 0.
{{ $.Comment }}minimum_mappable_gid = {{ .MinimumMappableGID}}

`

const templateStringCrioRuntimeCtrStopTimeout = `# The minimal amount of time in seconds to wait before issuing a timeout
# regarding the proper termination of the container. The lowest possible
# value is 30s, whereas lower values are not considered by CRI-O.
{{ $.Comment }}ctr_stop_timeout = {{ .CtrStopTimeout }}

`

const templateStringCrioRuntimeDropInfraCtr = `# drop_infra_ctr determines whether CRI-O drops the infra container
# when a pod does not have a private PID namespace, and does not use
# a kernel separating runtime (like kata).
# It requires manage_ns_lifecycle to be true.
{{ $.Comment }}drop_infra_ctr = {{ .DropInfraCtr }}

`

const templateStringCrioRuntimeInfraCtrCpuset = `# infra_ctr_cpuset determines what CPUs will be used to run infra containers.
# You can use linux CPU list format to specify desired CPUs.
# To get better isolation for guaranteed pods, set this parameter to be equal to kubelet reserved-cpus.
{{ $.Comment }}infra_ctr_cpuset = "{{ .InfraCtrCPUSet }}"

`

const templateStringCrioRuntimeNamespacesDir = `# The directory where the state of the managed namespaces gets tracked.
# Only used when manage_ns_lifecycle is true.
{{ $.Comment }}namespaces_dir = "{{ .NamespacesDir }}"

`

const templateStringCrioRuntimePinnsPath = `# pinns_path is the path to find the pinns binary, which is needed to manage namespace lifecycle
{{ $.Comment }}pinns_path = "{{ .PinnsPath }}"

`

const templateStringCrioRuntimeEnableCriuSupport = `# Globally enable/disable CRIU support which is necessary to
# checkpoint and restore container or pods (even if CRIU is found in $PATH).
{{ $.Comment }}enable_criu_support = {{ .EnableCriuSupport }}

`

const templateStringCrioRuntimeDefaultRuntime = `# default_runtime is the _name_ of the OCI runtime to be used as the default.
# default_runtime is the _name_ of the OCI runtime to be used as the default.
# The name is matched against the runtimes map below.
{{ $.Comment }}default_runtime = "{{ .DefaultRuntime }}"

`

const templateStringCrioRuntimeAbsentMountSourcesToReject = `# A list of paths that, when absent from the host,
# will cause a container creation to fail (as opposed to the current behavior being created as a directory).
# This option is to protect from source locations whose existence as a directory could jepordize the health of the node, and whose
# creation as a file is not desired either.
# An example is /etc/hostname, which will cause failures on reboot if it's created as a directory, but often doesn't exist because
# the hostname is being managed dynamically.
{{ $.Comment }}absent_mount_sources_to_reject = [
{{ range $mount := .AbsentMountSourcesToReject}}{{ $.Comment }}{{ printf "\t%q,\n" $mount}}{{ end }}{{ $.Comment }}]

`

const templateStringCrioRuntimeRuntimesRuntimeHandler = `# The "crio.runtime.runtimes" table defines a list of OCI compatible runtimes.
# The runtime to use is picked based on the runtime handler provided by the CRI.
# If no runtime handler is provided, the "default_runtime" will be used.
# Each entry in the table should follow the format:
#
# [crio.runtime.runtimes.runtime-handler]
# runtime_path = "/path/to/the/executable"
# runtime_type = "oci"
# runtime_root = "/path/to/the/root"
# monitor_path = "/path/to/container/monitor"
# monitor_cgroup = "/cgroup/path"
# monitor_exec_cgroup = "/cgroup/path"
# monitor_env = []
# privileged_without_host_devices = false
# allowed_annotations = []
# Where:
# - runtime-handler: Name used to identify the runtime.
# - runtime_path (optional, string): Absolute path to the runtime executable in
#   the host filesystem. If omitted, the runtime-handler identifier should match
#   the runtime executable name, and the runtime executable should be placed
#   in $PATH.
# - runtime_type (optional, string): Type of runtime, one of: "oci", "vm". If
#   omitted, an "oci" runtime is assumed.
# - runtime_root (optional, string): Root directory for storage of containers
#   state.
# - runtime_config_path (optional, string): the path for the runtime configuration
#   file. This can only be used with when using the VM runtime_type.
# - privileged_without_host_devices (optional, bool): an option for restricting
#   host devices from being passed to privileged containers.
# - allowed_annotations (optional, array of strings): an option for specifying
#   a list of experimental annotations that this runtime handler is allowed to process.
#   The currently recognized values are:
#   "io.kubernetes.cri-o.userns-mode" for configuring a user namespace for the pod.
#   "io.kubernetes.cri-o.cgroup2-mount-hierarchy-rw" for mounting cgroups writably when set to "true".
#   "io.kubernetes.cri-o.Devices" for configuring devices for the pod.
#   "io.kubernetes.cri-o.ShmSize" for configuring the size of /dev/shm.
#   "io.kubernetes.cri-o.UnifiedCgroup.$CTR_NAME" for configuring the cgroup v2 unified block for a container.
#   "io.containers.trace-syscall" for tracing syscalls via the OCI seccomp BPF hook.
#   "io.kubernetes.cri.rdt-class" for setting the RDT class of a container
# - monitor_path (optional, string): The path of the monitor binary. Replaces
#   deprecated option "conmon".
# - monitor_cgroup (optional, string): The cgroup the container monitor process will be put in.
#   Replaces deprecated option "conmon_cgroup".
# - monitor_exec_cgroup (optional, string): If set to "container", indicates exec probes
#   should be moved to the container's cgroup
# - monitor_env (optional, array of strings): Environment variables to pass to the montior.
#   Replaces deprecated option "conmon_env".
{{ range $runtime_name, $runtime_handler := .Runtimes  }}
{{ $.Comment }}[crio.runtime.runtimes.{{ $runtime_name }}]
{{ $.Comment }}runtime_path = "{{ $runtime_handler.RuntimePath }}"
{{ $.Comment }}runtime_type = "{{ $runtime_handler.RuntimeType }}"
{{ $.Comment }}runtime_root = "{{ $runtime_handler.RuntimeRoot }}"
{{ $.Comment }}runtime_config_path = "{{ $runtime_handler.RuntimeConfigPath }}"
{{ $.Comment }}monitor_path = "{{ $runtime_handler.MonitorPath }}"
{{ $.Comment }}monitor_cgroup = "{{ $runtime_handler.MonitorCgroup }}"
{{ $.Comment }}monitor_exec_cgroup = "{{ $runtime_handler.MonitorExecCgroup }}"
{{ $.Comment }}{{ if $runtime_handler.MonitorEnv }}monitor_env = [
{{ range $opt := $runtime_handler.MonitorEnv }}{{ $.Comment }}{{ printf "\t%q,\n" $opt }}{{ end }}{{ $.Comment }}]{{ end }}
{{ if $runtime_handler.AllowedAnnotations }}{{ $.Comment }}allowed_annotations = [
{{ range $opt := $runtime_handler.AllowedAnnotations }}{{ $.Comment }}{{ printf "\t%q,\n" $opt }}{{ end }}{{ $.Comment }}]{{ end }}
{{ $.Comment }}privileged_without_host_devices = {{ $runtime_handler.PrivilegedWithoutHostDevices }}
{{ end }}
`

const templateStringCrioRuntimeWorkloads = `# The workloads table defines ways to customize containers with different resources
# that work based on annotations, rather than the CRI.
# Note, the behavior of this table is EXPERIMENTAL and may change at any time.
# Each workload, has a name, activation_annotation, annotation_prefix and set of resources it supports mutating.
# The currently supported resources are "cpu" (to configure the cpu shares) and "cpuset" to configure the cpuset.
# Each resource can have a default value specified, or be empty.
# For a container to opt-into this workload, the pod should be configured with the annotation $activation_annotation (key only, value is ignored).
# To customize per-container, an annotation of the form $annotation_prefix.$resource/$ctrName = "value" can be specified
# signifying for that resource type to override the default value.
# If the annotation_prefix is not present, every container in the pod will be given the default values.
# Example:
# [crio.runtime.workloads.workload-type]
# activation_annotation = "io.crio/workload"
# annotation_prefix = "io.crio.workload-type"
# [crio.runtime.workloads.workload-type.resources]
# cpuset = 0
# cpushares = "0-1"
# Where:
# The workload name is workload-type.
# To specify, the pod must have the "io.crio.workload" annotation (this is a precise string match).
# This workload supports setting cpuset and cpu resources.
# annotation_prefix is used to customize the different resources.
# To configure the cpu shares a container gets in the example above, the pod would have to have the following annotation:
# "io.crio.workload-type/$container_name = {"cpushares": "value"}"
{{ range $workload_type, $workload_config := .Workloads  }}
{{ $.Comment }}[crio.runtime.workloads.{{ $workload_type }}]
{{ $.Comment }}activation_annotation = "{{ $workload_config.ActivationAnnotation }}"
{{ $.Comment }}annotation_prefix = "{{ $workload_config.AnnotationPrefix }}"
{{ $.Comment }}[crio.runtime.workloads.{{ $workload_type }}.resources]
{{ $.Comment }}cpuset = "{{ $workload_config.Resources.CPUSet }}"
{{ $.Comment }}cpushares = {{ $workload_config.Resources.CPUShares }}
{{ end }}
`

const templateStringCrioImage = `# The crio.image table contains settings pertaining to the management of OCI images.
#
# CRI-O reads its configured registries defaults from the system wide
# containers-registries.conf(5) located in /etc/containers/registries.conf. If
# you want to modify just CRI-O, you can change the registries configuration in
# this file. Otherwise, leave insecure_registries and registries commented out to
# use the system's defaults from /etc/containers/registries.conf.
[crio.image]

`

const templateStringCrioImageDefaultTransport = `# Default transport for pulling images from a remote container storage.
{{ $.Comment }}default_transport = "{{ .DefaultTransport }}"

`

const templateStringCrioImageGlobalAuthFile = `# The path to a file containing credentials necessary for pulling images from
# secure registries. The file is similar to that of /var/lib/kubelet/config.json
{{ $.Comment }}global_auth_file = "{{ .GlobalAuthFile }}"

`

const templateStringCrioImagePauseImage = `# The image used to instantiate infra containers.
# This option supports live configuration reload.
{{ $.Comment }}pause_image = "{{ .PauseImage }}"

`

const templateStringCrioImagePauseImageAuthFile = `# The path to a file containing credentials specific for pulling the pause_image from
# above. The file is similar to that of /var/lib/kubelet/config.json
# This option supports live configuration reload.
{{ $.Comment }}pause_image_auth_file = "{{ .PauseImageAuthFile }}"

`

const templateStringCrioImagePauseCommand = `# The command to run to have a container stay in the paused state.
# When explicitly set to "", it will fallback to the entrypoint and command
# specified in the pause image. When commented out, it will fallback to the
# default: "/pause". This option supports live configuration reload.
{{ $.Comment }}pause_command = "{{ .PauseCommand }}"

`

const templateStringCrioImageSignaturePolicy = `# Path to the file which decides what sort of policy we use when deciding
# whether or not to trust an image that we've pulled. It is not recommended that
# this option be used, as the default behavior of using the system-wide default
# policy (i.e., /etc/containers/policy.json) is most often preferred. Please
# refer to containers-policy.json(5) for more details.
{{ $.Comment }}signature_policy = "{{ .SignaturePolicyPath }}"

`

const templateStringCrioImageInsecureRegistries = `# List of registries to skip TLS verification for pulling images. Please
# consider configuring the registries via /etc/containers/registries.conf before
# changing them here.
{{ $.Comment }}insecure_registries = [
{{ range $opt := .InsecureRegistries }}{{ $.Comment }}{{ printf "\t%q,\n" $opt }}{{ end }}{{ $.Comment }}]

`

const templateStringCrioImageImageVolumes = `# Controls how image volumes are handled. The valid values are mkdir, bind and
# ignore; the latter will ignore volumes entirely.
{{ $.Comment }}image_volumes = "{{ .ImageVolumes }}"

`

const templateStringCrioImageBigFilesTemporaryDir = `# Temporary directory to use for storing big files
{{ $.Comment }}big_files_temporary_dir = "{{ .BigFilesTemporaryDir }}"

`

const templateStringCrioNetwork = `# The crio.network table containers settings pertaining to the management of
# CNI plugins.
[crio.network]

`

const templateStringCrioNetworkCniDefaultNetwork = `# The default CNI network name to be selected. If not set or "", then
# CRI-O will pick-up the first one found in network_dir.
# cni_default_network = "{{ .CNIDefaultNetwork }}"

`

const templateStringCrioNetworkNetworkDir = `# Path to the directory where CNI configuration files are located.
{{ $.Comment }}network_dir = "{{ .NetworkDir }}"

`

const templateStringCrioNetworkPluginDirs = `# Paths to directories where CNI plugin binaries are located.
{{ $.Comment }}plugin_dirs = [
{{ range $opt := .PluginDirs }}{{ $.Comment }}{{ printf "\t%q,\n" $opt }}{{ end }}{{ $.Comment }}]

`

const templateStringCrioMetrics = `# A necessary configuration for Prometheus based metrics retrieval
[crio.metrics]

`

const templateStringCrioMetricsEnableMetrics = `# Globally enable or disable metrics support.
{{ $.Comment }}enable_metrics = {{ .EnableMetrics }}

`

const templateStringCrioMetricsCollectors = `# Specify enabled metrics collectors.
# Per default all metrics are enabled.
# It is possible, to prefix the metrics with "container_runtime_" and "crio_".
# For example, the metrics collector "operations" would be treated in the same
# way as "crio_operations" and "container_runtime_crio_operations".
{{ $.Comment }}metrics_collectors = [
{{ range $opt := .MetricsCollectors }}{{ $.Comment }}{{ printf "\t%q,\n" $opt }}{{ end }}{{ $.Comment }}]
`

const templateStringCrioMetricsMetricsPort = `# The port on which the metrics server will listen.
{{ $.Comment }}metrics_port = {{ .MetricsPort }}

`

const templateStringCrioMetricsMetricsSocket = `# Local socket path to bind the metrics server to
{{ $.Comment }}metrics_socket = "{{ .MetricsSocket }}"

`

const templateStringCrioMetricsMetricsCert = `# The certificate for the secure metrics server.
# If the certificate is not available on disk, then CRI-O will generate a
# self-signed one. CRI-O also watches for changes of this path and reloads the
# certificate on any modification event.
{{ $.Comment }}metrics_cert = "{{ .MetricsCert }}"

`

const templateStringCrioMetricsMetricsKey = `# The certificate key for the secure metrics server.
# Behaves in the same way as the metrics_cert.
{{ $.Comment }}metrics_key = "{{ .MetricsKey }}"

`

const templateStringCrioTracing = `# A necessary configuration for OpenTelemetry trace data exporting
[crio.tracing]

`

const templateStringCrioTracingEnableTracing = `# Globally enable or disable exporting OpenTelemetry traces.
{{ $.Comment }}enable_tracing = {{ .EnableTracing }}

`

const templateStringCrioTracingTracingEndpoint = `# Address on which the gRPC trace collector listens on.
{{ $.Comment }}tracing_endpoint = "{{ .TracingEndpoint }}"

`

const templateStringCrioTracingTracingSamplingRatePerMillion = `# Number of samples to collect per million spans.
{{ $.Comment }}tracing_sampling_rate_per_million = {{ .TracingSamplingRatePerMillion }}

`

const templateStringCrioTracingTracingSampleAlways = `# Always enable sampling while ignoring tracing_sampling_rate_per_million.
{{ $.Comment }}tracing_sample_always = {{ .TracingSampleAlways }}

`

const templateStringCrioStats = `# Necessary information pertaining to container and pod stats reporting.
[crio.stats]

`

const templateStringCrioStatsStatsCollectionPeriod = `# The number of seconds between collecting pod and container stats.
# If set to 0, the stats are collected on-demand instead.
{{ $.Comment }}stats_collection_period = {{ .StatsCollectionPeriod }}

`
