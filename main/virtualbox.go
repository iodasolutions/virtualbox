package main

import (
	"github.com/iodasolutions/virtualbox"
	"github.com/iodasolutions/xbee-common/provider"
)

func main() {
	var p virtualbox.Provider
	var a virtualbox.Admin
	provider.Execute(p, a)
}
