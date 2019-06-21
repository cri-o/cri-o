package main

import (
	"fmt"
	"io/ioutil"

	"github.com/cri-o/cri-o/lib/config"
	"github.com/cri-o/cri-o/oci"
	"github.com/sirupsen/logrus"
)

func main() {
	output := `
#if !defined(CONFIG_H)
#define CONFIG_H

#define BUF_SIZE %d
#define STDIO_BUF_SIZE %d
#define DEFAULT_SOCKET_PATH "%s"

#endif // CONFIG_H
`
	if err := ioutil.WriteFile("config.h", []byte(fmt.Sprintf(output, oci.BufSize, oci.BufSize, config.ContainerAttachSocketDir)), 0700); err != nil {
		logrus.Fatal(err)
	}
}
