package server

type safeMountInfo struct {
}

func (s *safeMountInfo) Close() {}

func safeMountSubPath(mountPoint, subpath, runDir string) (s *safeMountInfo, err error) {
	return &safeMountInfo{}, nil
}
