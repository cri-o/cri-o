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

package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emicklei/go-restful/swagger"
	"github.com/go-openapi/spec"
	"github.com/pborman/uuid"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	openapicommon "k8s.io/apimachinery/pkg/openapi"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/authenticatorfactory"
	authenticatorunion "k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	authorizerunion "k8s.io/apiserver/pkg/authorization/union"
	genericapifilters "k8s.io/apiserver/pkg/endpoints/filters"
	apiopenapi "k8s.io/apiserver/pkg/endpoints/openapi"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic"
	genericfilters "k8s.io/apiserver/pkg/server/filters"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/apiserver/pkg/server/mux"
	"k8s.io/apiserver/pkg/server/routes"
	restclient "k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"

	_ "k8s.io/apiserver/pkg/apis/apiserver/install"
)

const (
	// DefaultLegacyAPIPrefix is where the the legacy APIs will be located.
	DefaultLegacyAPIPrefix = "/api"

	// APIGroupPrefix is where non-legacy API group will be located.
	APIGroupPrefix = "/apis"
)

// Config is a structure used to configure a GenericAPIServer.
// It's members are sorted roughly in order of importance for composers.
type Config struct {
	// SecureServingInfo is required to serve https
	SecureServingInfo *SecureServingInfo

	// LoopbackClientConfig is a config for a privileged loopback connection to the API server
	// This is required for proper functioning of the PostStartHooks on a GenericAPIServer
	LoopbackClientConfig *restclient.Config
	// Authenticator determines which subject is making the request
	Authenticator authenticator.Request
	// Authorizer determines whether the subject is allowed to make the request based only
	// on the RequestURI
	Authorizer authorizer.Authorizer
	// AdmissionControl performs deep inspection of a given request (including content)
	// to set values and determine whether its allowed
	AdmissionControl      admission.Interface
	CorsAllowedOriginList []string

	EnableSwaggerUI bool
	EnableIndex     bool
	EnableProfiling bool
	// Requires generic profiling enabled
	EnableContentionProfiling bool
	EnableMetrics             bool

	// Version will enable the /version endpoint if non-nil
	Version *version.Info
	// AuditWriter is the destination for audit logs.  If nil, they will not be written.
	AuditWriter io.Writer
	// SupportsBasicAuth indicates that's at least one Authenticator supports basic auth
	// If this is true, a basic auth challenge is returned on authentication failure
	// TODO(roberthbailey): Remove once the server no longer supports http basic auth.
	SupportsBasicAuth bool
	// ExternalAddress is the host name to use for external (public internet) facing URLs (e.g. Swagger)
	// Will default to a value based on secure serving info and available ipv4 IPs.
	ExternalAddress string

	//===========================================================================
	// Fields you probably don't care about changing
	//===========================================================================

	// BuildHandlerChainsFunc allows you to build custom handler chains by decorating the apiHandler.
	BuildHandlerChainsFunc func(apiHandler http.Handler, c *Config) (secure, insecure http.Handler)
	// DiscoveryAddresses is used to build the IPs pass to discovery.  If nil, the ExternalAddress is
	// always reported
	DiscoveryAddresses DiscoveryAddresses
	// The default set of healthz checks. There might be more added via AddHealthzChecks dynamically.
	HealthzChecks []healthz.HealthzChecker
	// LegacyAPIGroupPrefixes is used to set up URL parsing for authorization and for validating requests
	// to InstallLegacyAPIGroup.  New API servers don't generally have legacy groups at all.
	LegacyAPIGroupPrefixes sets.String
	// RequestContextMapper maps requests to contexts. Exported so downstream consumers can provider their own mappers
	// TODO confirm that anyone downstream actually uses this and doesn't just need an accessor
	RequestContextMapper apirequest.RequestContextMapper
	// Serializer is required and provides the interface for serializing and converting objects to and from the wire
	// The default (api.Codecs) usually works fine.
	Serializer runtime.NegotiatedSerializer
	// OpenAPIConfig will be used in generating OpenAPI spec. This is nil by default. Use DefaultOpenAPIConfig for "working" defaults.
	OpenAPIConfig *openapicommon.Config
	// SwaggerConfig will be used in generating Swagger spec. This is nil by default. Use DefaultSwaggerConfig for "working" defaults.
	SwaggerConfig *swagger.Config

	// RESTOptionsGetter is used to construct RESTStorage types via the generic registry.
	RESTOptionsGetter genericregistry.RESTOptionsGetter

	// If specified, requests will be allocated a random timeout between this value, and twice this value.
	// Note that it is up to the request handlers to ignore or honor this timeout. In seconds.
	MinRequestTimeout int
	// MaxRequestsInFlight is the maximum number of parallel non-long-running requests. Every further
	// request has to wait. Applies only to non-mutating requests.
	MaxRequestsInFlight int
	// MaxMutatingRequestsInFlight is the maximum number of parallel mutating requests. Every further
	// request has to wait.
	MaxMutatingRequestsInFlight int
	// Predicate which is true for paths of long-running http requests
	LongRunningFunc genericfilters.LongRunningRequestCheck

	// InsecureServingInfo is required to serve http.  HTTP does NOT include authentication or authorization.
	// You shouldn't be using this.  It makes sig-auth sad.
	InsecureServingInfo *ServingInfo

	//===========================================================================
	// values below here are targets for removal
	//===========================================================================

	// The port on PublicAddress where a read-write server will be installed.
	// Defaults to 6443 if not set.
	ReadWritePort int
	// PublicAddress is the IP address where members of the cluster (kubelet,
	// kube-proxy, services, etc.) can reach the GenericAPIServer.
	// If nil or 0.0.0.0, the host's default interface will be used.
	PublicAddress net.IP
}

type ServingInfo struct {
	// BindAddress is the ip:port to serve on
	BindAddress string
	// BindNetwork is the type of network to bind to - defaults to "tcp", accepts "tcp",
	// "tcp4", and "tcp6".
	BindNetwork string
}

type SecureServingInfo struct {
	ServingInfo

	// Cert is the main server cert which is used if SNI does not match. Cert must be non-nil and is
	// allowed to be in SNICerts.
	Cert *tls.Certificate

	// CACert is an optional certificate authority used for the loopback connection of the Admission controllers.
	// If this is nil, the certificate authority is extracted from Cert or a matching SNI certificate.
	CACert *tls.Certificate

	// SNICerts are the TLS certificates by name used for SNI.
	SNICerts map[string]*tls.Certificate

	// ClientCA is the certificate bundle for all the signers that you'll recognize for incoming client certificates
	ClientCA *x509.CertPool
}

// NewConfig returns a Config struct with the default values
func NewConfig() *Config {
	return &Config{
		ReadWritePort:               443,
		RequestContextMapper:        apirequest.NewRequestContextMapper(),
		BuildHandlerChainsFunc:      DefaultBuildHandlerChain,
		LegacyAPIGroupPrefixes:      sets.NewString(DefaultLegacyAPIPrefix),
		HealthzChecks:               []healthz.HealthzChecker{healthz.PingHealthz},
		EnableIndex:                 true,
		EnableProfiling:             true,
		MaxRequestsInFlight:         400,
		MaxMutatingRequestsInFlight: 200,
		MinRequestTimeout:           1800,

		// Default to treating watch as a long-running operation
		// Generic API servers have no inherent long-running subresources
		LongRunningFunc: genericfilters.BasicLongRunningRequestCheck(sets.NewString("watch"), sets.NewString()),
	}
}

func (c *Config) WithSerializer(codecs serializer.CodecFactory) *Config {
	c.Serializer = codecs
	return c
}

func DefaultOpenAPIConfig(getDefinitions openapicommon.GetOpenAPIDefinitions, scheme *runtime.Scheme) *openapicommon.Config {
	defNamer := apiopenapi.NewDefinitionNamer(scheme)
	return &openapicommon.Config{
		ProtocolList:   []string{"https"},
		IgnorePrefixes: []string{"/swaggerapi"},
		Info: &spec.Info{
			InfoProps: spec.InfoProps{
				Title: "Generic API Server",
			},
		},
		DefaultResponse: &spec.Response{
			ResponseProps: spec.ResponseProps{
				Description: "Default Response.",
			},
		},
		GetOperationIDAndTags: apiopenapi.GetOperationIDAndTags,
		GetDefinitionName:     defNamer.GetDefinitionName,
		GetDefinitions:        getDefinitions,
	}
}

// DefaultSwaggerConfig returns a default configuration without WebServiceURL and
// WebServices set.
func DefaultSwaggerConfig() *swagger.Config {
	return &swagger.Config{
		ApiPath:         "/swaggerapi/",
		SwaggerPath:     "/swaggerui/",
		SwaggerFilePath: "/swagger-ui/",
		SchemaFormatHandler: func(typeName string) string {
			switch typeName {
			case "metav1.Time", "*metav1.Time":
				return "date-time"
			}
			return ""
		},
	}
}

func (c *Config) ApplyClientCert(clientCAFile string) (*Config, error) {
	if c.SecureServingInfo != nil {
		if len(clientCAFile) > 0 {
			clientCAs, err := certutil.CertsFromFile(clientCAFile)
			if err != nil {
				return nil, fmt.Errorf("unable to load client CA file: %v", err)
			}
			if c.SecureServingInfo.ClientCA == nil {
				c.SecureServingInfo.ClientCA = x509.NewCertPool()
			}
			for _, cert := range clientCAs {
				c.SecureServingInfo.ClientCA.AddCert(cert)
			}
		}
	}

	return c, nil
}

type completedConfig struct {
	*Config
}

// Complete fills in any fields not set that are required to have valid data and can be derived
// from other fields.  If you're going to `ApplyOptions`, do that first.  It's mutating the receiver.
func (c *Config) Complete() completedConfig {
	if len(c.ExternalAddress) == 0 && c.PublicAddress != nil {
		hostAndPort := c.PublicAddress.String()
		if c.ReadWritePort != 0 {
			hostAndPort = net.JoinHostPort(hostAndPort, strconv.Itoa(c.ReadWritePort))
		}
		c.ExternalAddress = hostAndPort
	}
	if c.OpenAPIConfig != nil && c.OpenAPIConfig.SecurityDefinitions != nil {
		// Setup OpenAPI security: all APIs will have the same authentication for now.
		c.OpenAPIConfig.DefaultSecurity = []map[string][]string{}
		keys := []string{}
		for k := range *c.OpenAPIConfig.SecurityDefinitions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			c.OpenAPIConfig.DefaultSecurity = append(c.OpenAPIConfig.DefaultSecurity, map[string][]string{k: {}})
		}
		if c.OpenAPIConfig.CommonResponses == nil {
			c.OpenAPIConfig.CommonResponses = map[int]spec.Response{}
		}
		if _, exists := c.OpenAPIConfig.CommonResponses[http.StatusUnauthorized]; !exists {
			c.OpenAPIConfig.CommonResponses[http.StatusUnauthorized] = spec.Response{
				ResponseProps: spec.ResponseProps{
					Description: "Unauthorized",
				},
			}
		}
	}
	if c.SwaggerConfig != nil && len(c.SwaggerConfig.WebServicesUrl) == 0 {
		if c.SecureServingInfo != nil {
			c.SwaggerConfig.WebServicesUrl = "https://" + c.ExternalAddress
		} else {
			c.SwaggerConfig.WebServicesUrl = "http://" + c.ExternalAddress
		}
	}
	if c.DiscoveryAddresses == nil {
		c.DiscoveryAddresses = DefaultDiscoveryAddresses{DefaultAddress: c.ExternalAddress}
	}

	// If the loopbackclientconfig is specified AND it has a token for use against the API server
	// wrap the authenticator and authorizer in loopback authentication logic
	if c.Authenticator != nil && c.Authorizer != nil && c.LoopbackClientConfig != nil && len(c.LoopbackClientConfig.BearerToken) > 0 {
		privilegedLoopbackToken := c.LoopbackClientConfig.BearerToken
		var uid = uuid.NewRandom().String()
		tokens := make(map[string]*user.DefaultInfo)
		tokens[privilegedLoopbackToken] = &user.DefaultInfo{
			Name:   user.APIServerUser,
			UID:    uid,
			Groups: []string{user.SystemPrivilegedGroup},
		}

		tokenAuthenticator := authenticatorfactory.NewFromTokens(tokens)
		c.Authenticator = authenticatorunion.New(tokenAuthenticator, c.Authenticator)

		tokenAuthorizer := authorizerfactory.NewPrivilegedGroups(user.SystemPrivilegedGroup)
		c.Authorizer = authorizerunion.New(tokenAuthorizer, c.Authorizer)
	}

	return completedConfig{c}
}

// SkipComplete provides a way to construct a server instance without config completion.
func (c *Config) SkipComplete() completedConfig {
	return completedConfig{c}
}

// New returns a new instance of GenericAPIServer from the given config.
// Certain config fields will be set to a default value if unset,
// including:
//   ServiceClusterIPRange
//   ServiceNodePortRange
//   MasterCount
//   ReadWritePort
//   PublicAddress
// Public fields:
//   Handler -- The returned GenericAPIServer has a field TopHandler which is an
//   http.Handler which handles all the endpoints provided by the GenericAPIServer,
//   including the API, the UI, and miscellaneous debugging endpoints.  All
//   these are subject to authorization and authentication.
//   InsecureHandler -- an http.Handler which handles all the same
//   endpoints as Handler, but no authorization and authentication is done.
// Public methods:
//   HandleWithAuth -- Allows caller to add an http.Handler for an endpoint
//   that uses the same authentication and authorization (if any is configured)
//   as the GenericAPIServer's built-in endpoints.
//   If the caller wants to add additional endpoints not using the GenericAPIServer's
//   auth, then the caller should create a handler for those endpoints, which delegates the
//   any unhandled paths to "Handler".
func (c completedConfig) New() (*GenericAPIServer, error) {
	if c.Serializer == nil {
		return nil, fmt.Errorf("Genericapiserver.New() called with config.Serializer == nil")
	}
	if c.LoopbackClientConfig == nil {
		return nil, fmt.Errorf("Genericapiserver.New() called with config.LoopbackClientConfig == nil")
	}

	s := &GenericAPIServer{
		discoveryAddresses:     c.DiscoveryAddresses,
		LoopbackClientConfig:   c.LoopbackClientConfig,
		legacyAPIGroupPrefixes: c.LegacyAPIGroupPrefixes,
		admissionControl:       c.AdmissionControl,
		requestContextMapper:   c.RequestContextMapper,
		Serializer:             c.Serializer,

		minRequestTimeout: time.Duration(c.MinRequestTimeout) * time.Second,

		SecureServingInfo:   c.SecureServingInfo,
		InsecureServingInfo: c.InsecureServingInfo,
		ExternalAddress:     c.ExternalAddress,

		apiGroupsForDiscovery: map[string]metav1.APIGroup{},

		swaggerConfig: c.SwaggerConfig,
		openAPIConfig: c.OpenAPIConfig,

		postStartHooks: map[string]postStartHookEntry{},
		healthzChecks:  c.HealthzChecks,
	}

	s.HandlerContainer = mux.NewAPIContainer(http.NewServeMux(), c.Serializer)

	if s.openAPIConfig != nil {
		if s.openAPIConfig.Info == nil {
			s.openAPIConfig.Info = &spec.Info{}
		}
		if s.openAPIConfig.Info.Version == "" {
			if c.Version != nil {
				s.openAPIConfig.Info.Version = strings.Split(c.Version.String(), "-")[0]
			} else {
				s.openAPIConfig.Info.Version = "unversioned"
			}
		}
	}

	s.installAPI(c.Config)

	s.Handler, s.InsecureHandler = c.BuildHandlerChainsFunc(s.HandlerContainer.ServeMux, c.Config)

	return s, nil
}

func DefaultBuildHandlerChain(apiHandler http.Handler, c *Config) (secure, insecure http.Handler) {
	generic := func(handler http.Handler) http.Handler {
		handler = genericfilters.WithCORS(handler, c.CorsAllowedOriginList, nil, nil, nil, "true")
		handler = genericfilters.WithPanicRecovery(handler, c.RequestContextMapper)
		handler = genericfilters.WithTimeoutForNonLongRunningRequests(handler, c.RequestContextMapper, c.LongRunningFunc)
		handler = genericfilters.WithMaxInFlightLimit(handler, c.MaxRequestsInFlight, c.MaxMutatingRequestsInFlight, c.RequestContextMapper, c.LongRunningFunc)
		handler = genericapifilters.WithRequestInfo(handler, NewRequestInfoResolver(c), c.RequestContextMapper)
		handler = apirequest.WithRequestContext(handler, c.RequestContextMapper)
		return handler
	}
	audit := func(handler http.Handler) http.Handler {
		return genericapifilters.WithAudit(handler, c.RequestContextMapper, c.AuditWriter)
	}
	protect := func(handler http.Handler) http.Handler {
		handler = genericapifilters.WithAuthorization(handler, c.RequestContextMapper, c.Authorizer)
		handler = genericapifilters.WithImpersonation(handler, c.RequestContextMapper, c.Authorizer)
		handler = audit(handler) // before impersonation to read original user
		handler = genericapifilters.WithAuthentication(handler, c.RequestContextMapper, c.Authenticator, genericapifilters.Unauthorized(c.SupportsBasicAuth))
		return handler
	}

	return generic(protect(apiHandler)), generic(audit(apiHandler))
}

func (s *GenericAPIServer) installAPI(c *Config) {
	if c.EnableIndex {
		routes.Index{}.Install(s.HandlerContainer)
	}
	if c.SwaggerConfig != nil && c.EnableSwaggerUI {
		routes.SwaggerUI{}.Install(s.HandlerContainer)
	}
	if c.EnableProfiling {
		routes.Profiling{}.Install(s.HandlerContainer)
		if c.EnableContentionProfiling {
			goruntime.SetBlockProfileRate(1)
		}
	}
	if c.EnableMetrics {
		if c.EnableProfiling {
			routes.MetricsWithReset{}.Install(s.HandlerContainer)
		} else {
			routes.DefaultMetrics{}.Install(s.HandlerContainer)
		}
	}
	routes.Version{Version: c.Version}.Install(s.HandlerContainer)
	s.HandlerContainer.Add(s.DynamicApisDiscovery())
}

func NewRequestInfoResolver(c *Config) *apirequest.RequestInfoFactory {
	apiPrefixes := sets.NewString(strings.Trim(APIGroupPrefix, "/")) // all possible API prefixes
	legacyAPIPrefixes := sets.String{}                               // APIPrefixes that won't have groups (legacy)
	for legacyAPIPrefix := range c.LegacyAPIGroupPrefixes {
		apiPrefixes.Insert(strings.Trim(legacyAPIPrefix, "/"))
		legacyAPIPrefixes.Insert(strings.Trim(legacyAPIPrefix, "/"))
	}

	return &apirequest.RequestInfoFactory{
		APIPrefixes:          apiPrefixes,
		GrouplessAPIPrefixes: legacyAPIPrefixes,
	}
}
