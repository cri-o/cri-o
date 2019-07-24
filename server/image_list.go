package server

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/containers/libpod/libpod/image"
	"github.com/cri-o/cri-o/internal/pkg/log"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ListImages lists existing images.
func (s *Server) ListImages(ctx context.Context, req *pb.ListImagesRequest) (resp *pb.ListImagesResponse, err error) {
	const operation = "list_images"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	filter := ""
	reqFilter := req.GetFilter()
	if reqFilter != nil {
		filterImage := reqFilter.GetImage()
		if filterImage != nil {
			filter = filterImage.Image
		}
	}

	resp = &pb.ListImagesResponse{}
	images, err := s.ImageRuntime().GetImages()
	for _, img := range images {
		if RefMatchesImage(ctx, filter, img) {
			img := ConvertImage(ctx, img)
			resp.Images = append(resp.Images, img)
		}
	}

	return resp, nil
}

// RefMatchesImage checks if the provided ref matches any image name or ID.
// Empty ref's are automatically allowed.
func RefMatchesImage(ctx context.Context, ref string, img *image.Image) bool {
	if ref == "" {
		return true
	}

	filter := fmt.Sprintf("*%s*", ref)

	// Replacing all '/' with '|' so that filepath.Match() can work
	// '|' character is not valid in image name, so this is safe
	prepareRef := func(r string) string {
		return strings.Replace(r, "/", "|", -1) // nolint: gocritic
	}

	filter = prepareRef(filter)

	for _, name := range img.Names() {
		match, err := filepath.Match(filter, prepareRef(name))
		if err != nil {
			log.Errorf(ctx, "failed to match %s and %s, %q", name, ref, err)
		}
		if match {
			return true
		}
	}

	strippedRef := strings.Replace(ref, "@", "", -1) // nolint: gocritic
	return strings.Contains(img.ID(), strippedRef)
}

func ConvertImage(ctx context.Context, from *image.Image) *pb.Image {
	if from == nil {
		return nil
	}

	log.Debugf(ctx, "inspecting image: %+v", from)
	inspectData, err := from.Inspect(ctx)
	if err != nil {
		return nil
	}

	repoTags := []string{"<none>:<none>"}
	if len(inspectData.RepoTags) > 0 {
		repoTags = inspectData.RepoTags
	}

	repoDigests := []string{"<none>@<none>"}
	if len(inspectData.RepoDigests) > 0 {
		repoDigests = inspectData.RepoDigests
	}

	to := &pb.Image{
		Id:          inspectData.ID,
		RepoTags:    repoTags,
		RepoDigests: repoDigests,
	}

	uid, username := getUserFromImage(inspectData.User)
	to.Username = username

	if uid != nil {
		to.Uid = &pb.Int64Value{Value: *uid}
	}
	to.Size_ = uint64(inspectData.Size)

	return to
}
