package server

const (
	runtimeAPIVersion = "v1alpha1"
)

// Server implements the RuntimeService and ImageService
type Server struct {
	runtime    ociRuntime
	sandboxDir string
	sandboxes  []*sandbox
}

// New creates a new Server with options provided
func New(runtimePath, sandboxDir string) (*Server, error) {
	// TODO(runcom): runtimePath arg is unused but it might be useful
	// if we're willing to open the doors to other runtimes in the future.
	r := &runcRuntime{}
	return &Server{
		runtime:    r,
		sandboxDir: sandboxDir,
	}, nil
}

// TODO(runcom): export? this is being done just because runc shows a 3 line version :/
// but it might actually be a useful abstraction (?)
type ociRuntime interface {
	Name() string
	Path() (string, error)
	Version() (string, error)
}

type runcRuntime struct {
}

func (r *runcRuntime) Name() string {
	return "runc"
}

func (r *runcRuntime) Path() (string, error) {
	// TODO(runcom): we're saying runc is always in $PATH here for now
	return "runc", nil
}

func (r *runcRuntime) Version() (string, error) {
	path, err := r.Path()
	if err != nil {
		return "", err
	}
	runtimeVersion, err := execRuncVersion(path, "-v")
	if err != nil {
		return "", err
	}
	return runtimeVersion, nil
}

type sandbox struct {
	name   string
	logDir string
	labels map[string]string
}
