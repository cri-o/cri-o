package main

import (
	"os/user"
	"testing"

	is "github.com/containers/image/storage"
)

func TestImportImagePushDataFromImage(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Log("Could not determine user.  Running as root may cause tests to fail")
	} else if u.Uid != "0" {
		t.Fatal("tests will fail unless run as root")
	}
	// Get Store
	store, err := getStoreForTests()
	if err != nil {
		t.Fatalf("could not get store: %q", err)
	}
	// Pull an image and save it to the store
	testImageName := "docker.io/library/busybox:1.26"
	err = pullTestImage(testImageName)
	if err != nil {
		t.Fatalf("could not pull test image: %q", err)
	}
	img, err := findImage(store, testImageName)
	if err != nil {
		t.Fatalf("could not find image in store: %q", err)
	}
	// Get System Context
	systemContext := getSystemContext("")
	// Call importImagePushDataFromImage
	ipd, err := importImagePushDataFromImage(store, img, systemContext)
	if err != nil {
		t.Fatalf("could not get ImagePushData: %q", err)
	}
	// Get ref and from it, get the config and the manifest
	ref, err := is.Transport.ParseStoreReference(store, "@"+img.ID)
	if err != nil {
		t.Fatalf("no such image %q", "@"+img.ID)
	}
	src, err := ref.NewImage(systemContext)
	if err != nil {
		t.Fatalf("error creating new image from system context: %q", err)
	}
	defer src.Close()
	config, err := src.ConfigBlob()
	if err != nil {
		t.Fatalf("error reading image config: %q", err)
	}
	manifest, _, err := src.Manifest()
	if err != nil {
		t.Fatalf("error reading image manifest: %q", err)
	}
	//Create "expected" ipd struct
	expectedIpd := &imagePushData{
		store:            store,
		FromImage:        testImageName,
		FromImageID:      img.ID,
		Config:           config,
		Manifest:         manifest,
		ImageAnnotations: map[string]string{},
		ImageCreatedBy:   "",
	}
	expectedIpd.initConfig()
	//Compare structs, error if they are not the same
	if !compareImagePushData(ipd, expectedIpd) {
		t.Errorf("imagePushData did not match expected imagePushData")
	}
}
