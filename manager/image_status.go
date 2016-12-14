package manager

import pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"

// ImageStatus returns the status of the image.
func (m *Manager) ImageStatus(imageSpec *pb.ImageSpec) (*pb.Image, error) {
	// TODO
	// containers/storage will take care of this by looking inside /var/lib/ocid/images
	// and getting the image status
	return nil, nil
}
