package properties

import (
	"fmt"
	"github.com/iodasolutions/xbee-common/newfs"
	"os"
)

func VboxPath() string {
	root := os.Getenv("VBOX_MSI_INSTALL_PATH")
	if root == "" {
		panic(fmt.Errorf("On windows, env VBOX_MSI_INSTALL_PATH should be set"))
	}
	rootDir := newfs.NewFolder(root)
	return rootDir.ChildFile("VBoxManage.exe").String()
}
