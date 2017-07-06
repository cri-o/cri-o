package main

import (
	"os/user"
	"testing"

	"flag"

	"github.com/urfave/cli"
)

func TestGetStore(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Log("Could not determine user.  Running as root may cause tests to fail")
	} else if u.Uid != "0" {
		t.Fatal("tests will fail unless run as root")
	}

	set := flag.NewFlagSet("test", 0)
	globalSet := flag.NewFlagSet("test", 0)
	globalSet.String("root", "", "path to the root directory in which data, including images,  is stored")
	globalCtx := cli.NewContext(nil, globalSet, nil)
	command := cli.Command{Name: "imagesCommand"}
	c := cli.NewContext(nil, set, globalCtx)
	c.Command = command

	_, err = getStore(c)
	if err != nil {
		t.Error(err)
	}
}
