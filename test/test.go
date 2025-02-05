package main

import (
	"github.com/iodasolutions/xbee-common/newfs"
	"log"
)

func main() {
	ovafile := newfs.NewFile("/tmp/vm_export.ova")
	targetDir := newfs.NewFolder("/tmp/toto")
	err := ovafile.Untar(targetDir.String())
	if err != nil {
		log.Fatal(err)
	}
}
