package libpod

// State is a storage backend for libpod's current state
type State interface {
	// Accepts full ID of container
	GetContainer(id string) (*Container, error)
	// Accepts full or partial IDs (as long as they are unique) and names
	LookupContainer(idOrName string) (*Container, error)
	// Checks if a container with the given ID is present in the state
	HasContainer(id string) (bool, error)
	// Adds container to state
	// If the container belongs to a pod, that pod must already be present
	// in the state when the container is added
	AddContainer(ctr *Container) error
	// Removes container from state
	// If the container belongs to a pod, it will be removed from the pod
	// as well
	RemoveContainer(ctr *Container) error
	// Retrieves all containers presently in state
	GetAllContainers() ([]*Container, error)

	// Accepts full ID of pod
	GetPod(id string) (*Pod, error)
	// Accepts full or partial IDs (as long as they are unique) and names
	LookupPod(idOrName string) (*Pod, error)
	// Checks if a pod with the given ID is present in the state
	HasPod(id string) (bool, error)
	// Adds pod to state
	// Any containers within the pod not already in the state will be added
	// with it
	AddPod(pod *Pod) error
	// Removes pod from state
	// All containers within the pod will also be removed
	RemovePod(pod *Pod) error
	// Retrieves all pods presently in state
	GetAllPods() ([]*Pod, error)
}
