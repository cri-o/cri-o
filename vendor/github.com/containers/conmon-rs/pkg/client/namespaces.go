package client

import (
	"context"
	"fmt"
	"slices"

	"github.com/containers/conmon-rs/internal/proto"
	"go.podman.io/storage/pkg/idtools"
)

// CreateNamespacesConfig is the configuration for calling the
// CreateNamespaces method.
type CreateNamespacesConfig struct {
	// Namespaces are the list of namespaces to unshare. A namespace listed
	// twice is created and reported once.
	Namespaces []Namespace

	// IDMappings are the user and group ID mappings when unsharing the user
	// namespace.
	IDMappings *idtools.IDMappings

	// BasePath is the root path for storing the namespaces, it has to be
	// absolute. Defaults to "/var/run" if unset.
	BasePath string

	// PodID is the unique identifier of the pod. It names the files of the
	// namespaces below BasePath, so it must not be empty, may only contain
	// the characters A-Z, a-z, 0-9, "_", "." and "-", must not be "." or
	// ".." and takes 251 bytes at most. Servers since v1.2.1 reject other
	// values of BasePath and PodID.
	PodID string
}

// CreateNamespacesResponse is the response of the CreateNamespaces method.
type CreateNamespacesResponse struct {
	// Namespaces are the created namespaces, one per requested type. A
	// server holds the namespaces of a single pod and answers later calls
	// for it from the first one, so the order is that of the first request.
	// Match the entries by their Type.
	Namespaces []*NamespacesResponse
}

// NamespacesResponse is the response data for the CreateNamespacesResponse.
type NamespacesResponse struct {
	// Type is the namespace which got created.
	Type Namespace

	// Path is the bind mount of the namespace, which can be joined via
	// setns(2).
	Path string
}

// CreateNamespaces can be used to create a new set of unshared namespaces by
// bind mounting it to the local filesystem.
//
// Only the namespaces selected by the CreateNamespacesConfig are created and
// returned, in the requested order and without duplicates. They are mounted to
// $BASE_PATH/[ipc,pid,net,user,uts]ns/$POD_ID, where BASE_PATH and POD_ID are
// taken from the CreateNamespacesConfig. A pause process only stays around as
// init of a requested PID namespace, all other namespaces are kept alive by
// their bind mounts.
//
// UID and GID mappings are required if unsharing of the user namespace is
// requested.
func (c *ConmonClient) CreateNamespaces(
	ctx context.Context, cfg *CreateNamespacesConfig,
) (_ *CreateNamespacesResponse, retErr error) {
	ctx, span := c.startSpan(ctx, "CreateNamespaces")
	defer endSpan(span, &retErr)

	if cfg == nil {
		return nil, ErrMissingConfig
	}

	namespaces, err := namespacesToProto(cfg)
	if err != nil {
		return nil, err
	}

	basePath := cfg.BasePath
	if basePath == "" {
		basePath = "/var/run"
	}

	return call(ctx, c, proto.Conmon.CreateNamespaces,
		func(req proto.Conmon_CreateNamespacesRequest) error {
			namespaceList, err := req.NewNamespaces(int32(len(namespaces)))
			if err != nil {
				return fmt.Errorf("init namespaces: %w", err)
			}

			for i, namespace := range namespaces {
				namespaceList.Set(i, namespace)
			}

			if slices.Contains(namespaces, proto.Conmon_Namespace_user) {
				if err := stringSliceToTextList(
					mappingsToSlice(cfg.IDMappings.UIDs()),
					req.NewUidMappings,
				); err != nil {
					return fmt.Errorf("convert user ID mappings to text list: %w", err)
				}

				if err := stringSliceToTextList(
					mappingsToSlice(cfg.IDMappings.GIDs()),
					req.NewGidMappings,
				); err != nil {
					return fmt.Errorf("convert group ID mappings to text list: %w", err)
				}
			}

			if err := req.SetBasePath(basePath); err != nil {
				return fmt.Errorf("set base path: %w", err)
			}

			if err := req.SetPodId(cfg.PodID); err != nil {
				return fmt.Errorf("set pod ID: %w", err)
			}

			return nil
		},
		readCreateNamespacesResponse,
	)
}

// readCreateNamespacesResponse converts the response of a CreateNamespaces
// call.
func readCreateNamespacesResponse(
	response proto.Conmon_CreateNamespacesResponse,
) (*CreateNamespacesResponse, error) {
	responseNamespaces, err := response.Namespaces()
	if err != nil {
		return nil, fmt.Errorf("get namespaces: %w", err)
	}

	namespacesResponse := make([]*NamespacesResponse, 0, responseNamespaces.Len())

	for i := range responseNamespaces.Len() {
		namespace := responseNamespaces.At(i)

		typ, err := namespaceTypes.fromProto(namespace.Type())
		if err != nil {
			return nil, fmt.Errorf("namespace: %w", err)
		}

		path, err := namespace.Path()
		if err != nil {
			return nil, fmt.Errorf("namespace has no path: %w", err)
		}

		namespacesResponse = append(namespacesResponse,
			&NamespacesResponse{
				Type: typ,
				Path: path,
			},
		)
	}

	return &CreateNamespacesResponse{
		Namespaces: namespacesResponse,
	}, nil
}

// namespacesToProto converts the requested namespaces and checks that a
// user namespace comes with ID mappings.
func namespacesToProto(cfg *CreateNamespacesConfig) ([]proto.Conmon_Namespace, error) {
	namespaces := make([]proto.Conmon_Namespace, 0, len(cfg.Namespaces))

	for _, namespace := range cfg.Namespaces {
		typ, err := namespaceTypes.toProto(namespace)
		if err != nil {
			return nil, fmt.Errorf("namespace: %w", err)
		}

		if typ == proto.Conmon_Namespace_user && (cfg.IDMappings == nil ||
			len(cfg.IDMappings.UIDs()) == 0 ||
			len(cfg.IDMappings.GIDs()) == 0) {
			return nil, ErrMissingIDMappings
		}

		namespaces = append(namespaces, typ)
	}

	return namespaces, nil
}

// namespaceTypes maps the namespaces to their protocol values.
var namespaceTypes = enumTable[Namespace, proto.Conmon_Namespace]{
	{NamespaceIPC, proto.Conmon_Namespace_ipc},
	{NamespacePID, proto.Conmon_Namespace_pid},
	{NamespaceNet, proto.Conmon_Namespace_net},
	{NamespaceUser, proto.Conmon_Namespace_user},
	{NamespaceUTS, proto.Conmon_Namespace_uts},
}

func mappingsToSlice(mappings []idtools.IDMap) []string {
	res := make([]string, 0, len(mappings))
	for _, m := range mappings {
		res = append(res, fmt.Sprintf("%d %d %d", m.ContainerID, m.HostID, m.Size))
	}

	return res
}
