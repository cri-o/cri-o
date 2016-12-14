package manager

import pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"

// ListImages lists existing images.
func (m *Manager) ListImages(filter *pb.ImageFilter) ([]*pb.Image, error) {
	// TODO
	// containers/storage will take care of this by looking inside /var/lib/ocid/images
	// and listing images.
	return nil, nil
}
