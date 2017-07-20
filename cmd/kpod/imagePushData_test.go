package main

import (
	"bytes"
	"fmt"
)

// We have to compare the structs manually because they contain
// []byte variables, which cannot be compared with "=="
func compareImagePushData(a, b *imagePushData) bool {
	if a.store != b.store {
		fmt.Println("store")
		return false
	} else if a.Type != b.Type {
		fmt.Println("type")
		return false
	} else if a.FromImage != b.FromImage {
		fmt.Println("FromImage")
		return false
	} else if a.FromImageID != b.FromImageID {
		fmt.Println("FromImageID")
		return false
	} else if !bytes.Equal(a.Config, b.Config) {
		fmt.Println("Config")
		return false
	} else if !bytes.Equal(a.Manifest, b.Manifest) {
		fmt.Println("Manifest")
		return false
	} else if fmt.Sprint(a.ImageAnnotations) != fmt.Sprint(b.ImageAnnotations) {
		fmt.Println("Annotations")
		return false
	} else if a.ImageCreatedBy != b.ImageCreatedBy {
		fmt.Println("ImageCreatedBy")
		return false
	} else if fmt.Sprintf("%+v", a.OCIv1) != fmt.Sprintf("%+v", b.OCIv1) {
		fmt.Println("OCIv1")
		return false
	}
	return true
}
