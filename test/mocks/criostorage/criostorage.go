// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/cri-o/cri-o/internal/storage (interfaces: ImageServer,RuntimeServer,StorageTransport)
//
// Generated by this command:
//
//	mockgen -package criostoragemock -destination ./test/mocks/criostorage/criostorage.go github.com/cri-o/cri-o/internal/storage ImageServer,RuntimeServer,StorageTransport
//

// Package criostoragemock is a generated GoMock package.
package criostoragemock

import (
	context "context"
	reflect "reflect"

	types "github.com/containers/image/v5/types"
	storage "github.com/containers/storage"
	storage0 "github.com/cri-o/cri-o/internal/storage"
	gomock "go.uber.org/mock/gomock"
)

// MockImageServer is a mock of ImageServer interface.
type MockImageServer struct {
	ctrl     *gomock.Controller
	recorder *MockImageServerMockRecorder
	isgomock struct{}
}

// MockImageServerMockRecorder is the mock recorder for MockImageServer.
type MockImageServerMockRecorder struct {
	mock *MockImageServer
}

// NewMockImageServer creates a new mock instance.
func NewMockImageServer(ctrl *gomock.Controller) *MockImageServer {
	mock := &MockImageServer{ctrl: ctrl}
	mock.recorder = &MockImageServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockImageServer) EXPECT() *MockImageServerMockRecorder {
	return m.recorder
}

// CandidatesForPotentiallyShortImageName mocks base method.
func (m *MockImageServer) CandidatesForPotentiallyShortImageName(systemContext *types.SystemContext, imageName string) ([]storage0.RegistryImageReference, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CandidatesForPotentiallyShortImageName", systemContext, imageName)
	ret0, _ := ret[0].([]storage0.RegistryImageReference)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CandidatesForPotentiallyShortImageName indicates an expected call of CandidatesForPotentiallyShortImageName.
func (mr *MockImageServerMockRecorder) CandidatesForPotentiallyShortImageName(systemContext, imageName any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CandidatesForPotentiallyShortImageName", reflect.TypeOf((*MockImageServer)(nil).CandidatesForPotentiallyShortImageName), systemContext, imageName)
}

// DeleteImage mocks base method.
func (m *MockImageServer) DeleteImage(systemContext *types.SystemContext, id storage0.StorageImageID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteImage", systemContext, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteImage indicates an expected call of DeleteImage.
func (mr *MockImageServerMockRecorder) DeleteImage(systemContext, id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteImage", reflect.TypeOf((*MockImageServer)(nil).DeleteImage), systemContext, id)
}

// GetStore mocks base method.
func (m *MockImageServer) GetStore() storage.Store {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetStore")
	ret0, _ := ret[0].(storage.Store)
	return ret0
}

// GetStore indicates an expected call of GetStore.
func (mr *MockImageServerMockRecorder) GetStore() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetStore", reflect.TypeOf((*MockImageServer)(nil).GetStore))
}

// HeuristicallyTryResolvingStringAsIDPrefix mocks base method.
func (m *MockImageServer) HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput string) *storage0.StorageImageID {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HeuristicallyTryResolvingStringAsIDPrefix", heuristicInput)
	ret0, _ := ret[0].(*storage0.StorageImageID)
	return ret0
}

// HeuristicallyTryResolvingStringAsIDPrefix indicates an expected call of HeuristicallyTryResolvingStringAsIDPrefix.
func (mr *MockImageServerMockRecorder) HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HeuristicallyTryResolvingStringAsIDPrefix", reflect.TypeOf((*MockImageServer)(nil).HeuristicallyTryResolvingStringAsIDPrefix), heuristicInput)
}

// ImageStatusByID mocks base method.
func (m *MockImageServer) ImageStatusByID(systemContext *types.SystemContext, id storage0.StorageImageID) (*storage0.ImageResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ImageStatusByID", systemContext, id)
	ret0, _ := ret[0].(*storage0.ImageResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ImageStatusByID indicates an expected call of ImageStatusByID.
func (mr *MockImageServerMockRecorder) ImageStatusByID(systemContext, id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ImageStatusByID", reflect.TypeOf((*MockImageServer)(nil).ImageStatusByID), systemContext, id)
}

// ImageStatusByName mocks base method.
func (m *MockImageServer) ImageStatusByName(systemContext *types.SystemContext, name storage0.RegistryImageReference) (*storage0.ImageResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ImageStatusByName", systemContext, name)
	ret0, _ := ret[0].(*storage0.ImageResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ImageStatusByName indicates an expected call of ImageStatusByName.
func (mr *MockImageServerMockRecorder) ImageStatusByName(systemContext, name any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ImageStatusByName", reflect.TypeOf((*MockImageServer)(nil).ImageStatusByName), systemContext, name)
}

// IsRunningImageAllowed mocks base method.
func (m *MockImageServer) IsRunningImageAllowed(ctx context.Context, systemContext *types.SystemContext, userSpecifiedImage storage0.RegistryImageReference, imageID storage0.StorageImageID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsRunningImageAllowed", ctx, systemContext, userSpecifiedImage, imageID)
	ret0, _ := ret[0].(error)
	return ret0
}

// IsRunningImageAllowed indicates an expected call of IsRunningImageAllowed.
func (mr *MockImageServerMockRecorder) IsRunningImageAllowed(ctx, systemContext, userSpecifiedImage, imageID any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsRunningImageAllowed", reflect.TypeOf((*MockImageServer)(nil).IsRunningImageAllowed), ctx, systemContext, userSpecifiedImage, imageID)
}

// ListImages mocks base method.
func (m *MockImageServer) ListImages(systemContext *types.SystemContext) ([]storage0.ImageResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListImages", systemContext)
	ret0, _ := ret[0].([]storage0.ImageResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListImages indicates an expected call of ListImages.
func (mr *MockImageServerMockRecorder) ListImages(systemContext any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListImages", reflect.TypeOf((*MockImageServer)(nil).ListImages), systemContext)
}

// PullImage mocks base method.
func (m *MockImageServer) PullImage(ctx context.Context, imageName storage0.RegistryImageReference, options *storage0.ImageCopyOptions) (storage0.RegistryImageReference, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PullImage", ctx, imageName, options)
	ret0, _ := ret[0].(storage0.RegistryImageReference)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PullImage indicates an expected call of PullImage.
func (mr *MockImageServerMockRecorder) PullImage(ctx, imageName, options any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PullImage", reflect.TypeOf((*MockImageServer)(nil).PullImage), ctx, imageName, options)
}

// UntagImage mocks base method.
func (m *MockImageServer) UntagImage(systemContext *types.SystemContext, name storage0.RegistryImageReference) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UntagImage", systemContext, name)
	ret0, _ := ret[0].(error)
	return ret0
}

// UntagImage indicates an expected call of UntagImage.
func (mr *MockImageServerMockRecorder) UntagImage(systemContext, name any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UntagImage", reflect.TypeOf((*MockImageServer)(nil).UntagImage), systemContext, name)
}

// UpdatePinnedImagesList mocks base method.
func (m *MockImageServer) UpdatePinnedImagesList(imageList []string) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "UpdatePinnedImagesList", imageList)
}

// UpdatePinnedImagesList indicates an expected call of UpdatePinnedImagesList.
func (mr *MockImageServerMockRecorder) UpdatePinnedImagesList(imageList any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdatePinnedImagesList", reflect.TypeOf((*MockImageServer)(nil).UpdatePinnedImagesList), imageList)
}

// MockRuntimeServer is a mock of RuntimeServer interface.
type MockRuntimeServer struct {
	ctrl     *gomock.Controller
	recorder *MockRuntimeServerMockRecorder
	isgomock struct{}
}

// MockRuntimeServerMockRecorder is the mock recorder for MockRuntimeServer.
type MockRuntimeServerMockRecorder struct {
	mock *MockRuntimeServer
}

// NewMockRuntimeServer creates a new mock instance.
func NewMockRuntimeServer(ctrl *gomock.Controller) *MockRuntimeServer {
	mock := &MockRuntimeServer{ctrl: ctrl}
	mock.recorder = &MockRuntimeServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRuntimeServer) EXPECT() *MockRuntimeServerMockRecorder {
	return m.recorder
}

// CreateContainer mocks base method.
func (m *MockRuntimeServer) CreateContainer(systemContext *types.SystemContext, podName, podID, userRequestedImage string, imageID storage0.StorageImageID, containerName, containerID, metadataName string, attempt uint32, idMappingsOptions *storage.IDMappingOptions, labelOptions []string, privileged bool) (storage0.ContainerInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateContainer", systemContext, podName, podID, userRequestedImage, imageID, containerName, containerID, metadataName, attempt, idMappingsOptions, labelOptions, privileged)
	ret0, _ := ret[0].(storage0.ContainerInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateContainer indicates an expected call of CreateContainer.
func (mr *MockRuntimeServerMockRecorder) CreateContainer(systemContext, podName, podID, userRequestedImage, imageID, containerName, containerID, metadataName, attempt, idMappingsOptions, labelOptions, privileged any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateContainer", reflect.TypeOf((*MockRuntimeServer)(nil).CreateContainer), systemContext, podName, podID, userRequestedImage, imageID, containerName, containerID, metadataName, attempt, idMappingsOptions, labelOptions, privileged)
}

// CreatePodSandbox mocks base method.
func (m *MockRuntimeServer) CreatePodSandbox(systemContext *types.SystemContext, podName, podID string, pauseImage storage0.RegistryImageReference, imageAuthFile, containerName, metadataName, uid, namespace string, attempt uint32, idMappingsOptions *storage.IDMappingOptions, labelOptions []string, privileged bool) (storage0.ContainerInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreatePodSandbox", systemContext, podName, podID, pauseImage, imageAuthFile, containerName, metadataName, uid, namespace, attempt, idMappingsOptions, labelOptions, privileged)
	ret0, _ := ret[0].(storage0.ContainerInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreatePodSandbox indicates an expected call of CreatePodSandbox.
func (mr *MockRuntimeServerMockRecorder) CreatePodSandbox(systemContext, podName, podID, pauseImage, imageAuthFile, containerName, metadataName, uid, namespace, attempt, idMappingsOptions, labelOptions, privileged any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreatePodSandbox", reflect.TypeOf((*MockRuntimeServer)(nil).CreatePodSandbox), systemContext, podName, podID, pauseImage, imageAuthFile, containerName, metadataName, uid, namespace, attempt, idMappingsOptions, labelOptions, privileged)
}

// DeleteContainer mocks base method.
func (m *MockRuntimeServer) DeleteContainer(ctx context.Context, idOrName string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteContainer", ctx, idOrName)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteContainer indicates an expected call of DeleteContainer.
func (mr *MockRuntimeServerMockRecorder) DeleteContainer(ctx, idOrName any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteContainer", reflect.TypeOf((*MockRuntimeServer)(nil).DeleteContainer), ctx, idOrName)
}

// GetContainerMetadata mocks base method.
func (m *MockRuntimeServer) GetContainerMetadata(idOrName string) (storage0.RuntimeContainerMetadata, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetContainerMetadata", idOrName)
	ret0, _ := ret[0].(storage0.RuntimeContainerMetadata)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetContainerMetadata indicates an expected call of GetContainerMetadata.
func (mr *MockRuntimeServerMockRecorder) GetContainerMetadata(idOrName any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetContainerMetadata", reflect.TypeOf((*MockRuntimeServer)(nil).GetContainerMetadata), idOrName)
}

// GetRunDir mocks base method.
func (m *MockRuntimeServer) GetRunDir(id string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRunDir", id)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRunDir indicates an expected call of GetRunDir.
func (mr *MockRuntimeServerMockRecorder) GetRunDir(id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRunDir", reflect.TypeOf((*MockRuntimeServer)(nil).GetRunDir), id)
}

// GetWorkDir mocks base method.
func (m *MockRuntimeServer) GetWorkDir(id string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetWorkDir", id)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetWorkDir indicates an expected call of GetWorkDir.
func (mr *MockRuntimeServerMockRecorder) GetWorkDir(id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetWorkDir", reflect.TypeOf((*MockRuntimeServer)(nil).GetWorkDir), id)
}

// SetContainerMetadata mocks base method.
func (m *MockRuntimeServer) SetContainerMetadata(idOrName string, metadata *storage0.RuntimeContainerMetadata) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetContainerMetadata", idOrName, metadata)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetContainerMetadata indicates an expected call of SetContainerMetadata.
func (mr *MockRuntimeServerMockRecorder) SetContainerMetadata(idOrName, metadata any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetContainerMetadata", reflect.TypeOf((*MockRuntimeServer)(nil).SetContainerMetadata), idOrName, metadata)
}

// StartContainer mocks base method.
func (m *MockRuntimeServer) StartContainer(idOrName string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "StartContainer", idOrName)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// StartContainer indicates an expected call of StartContainer.
func (mr *MockRuntimeServerMockRecorder) StartContainer(idOrName any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "StartContainer", reflect.TypeOf((*MockRuntimeServer)(nil).StartContainer), idOrName)
}

// StopContainer mocks base method.
func (m *MockRuntimeServer) StopContainer(ctx context.Context, idOrName string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "StopContainer", ctx, idOrName)
	ret0, _ := ret[0].(error)
	return ret0
}

// StopContainer indicates an expected call of StopContainer.
func (mr *MockRuntimeServerMockRecorder) StopContainer(ctx, idOrName any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "StopContainer", reflect.TypeOf((*MockRuntimeServer)(nil).StopContainer), ctx, idOrName)
}

// MockStorageTransport is a mock of StorageTransport interface.
type MockStorageTransport struct {
	ctrl     *gomock.Controller
	recorder *MockStorageTransportMockRecorder
	isgomock struct{}
}

// MockStorageTransportMockRecorder is the mock recorder for MockStorageTransport.
type MockStorageTransportMockRecorder struct {
	mock *MockStorageTransport
}

// NewMockStorageTransport creates a new mock instance.
func NewMockStorageTransport(ctrl *gomock.Controller) *MockStorageTransport {
	mock := &MockStorageTransport{ctrl: ctrl}
	mock.recorder = &MockStorageTransportMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStorageTransport) EXPECT() *MockStorageTransportMockRecorder {
	return m.recorder
}

// ResolveReference mocks base method.
func (m *MockStorageTransport) ResolveReference(ref types.ImageReference) (types.ImageReference, *storage.Image, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ResolveReference", ref)
	ret0, _ := ret[0].(types.ImageReference)
	ret1, _ := ret[1].(*storage.Image)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ResolveReference indicates an expected call of ResolveReference.
func (mr *MockStorageTransportMockRecorder) ResolveReference(ref any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ResolveReference", reflect.TypeOf((*MockStorageTransport)(nil).ResolveReference), ref)
}
