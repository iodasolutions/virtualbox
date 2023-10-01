package properties

import (
	"github.com/iodasolutions/xbee-common/exec2"
	"github.com/iodasolutions/xbee-common/newfs"
	"github.com/iodasolutions/xbee-common/stringutils"
	"github.com/iodasolutions/xbee-common/util"
	"strings"
	"sync"
)

var system struct {
	properties map[string]string
	once       sync.Once
}

func initSystem() map[string]string {
	result := map[string]string{}
	if err := exec2.Run(nil, VboxPath(), "list", "systemproperties"); err != nil {
		panic(err)
	}
	if out, err := exec2.RunReturnStdOut(nil, VboxPath(), "list", "systemproperties"); err != nil {
		panic(err)
	} else {
		for _, line := range stringutils.Split(out) {
			sLine := line.(string)
			colonIndex := strings.Index(sLine, ":")
			name := sLine[0:colonIndex]
			value := strings.TrimSpace(sLine[colonIndex+1:])
			result[name] = value
		}
	}
	return result
}

func defaultMachineFolder() string {
	return fromSystem("Default machine folder")
}

func VmFolder(name string) newfs.Folder {
	return newfs.Folder(defaultMachineFolder()).ChildFolder(name)
}

func fromSystem(key string) string {
	system.once.Do(func() {
		system.properties = initSystem()
	})
	if value, ok := system.properties[key]; ok {
		return value
	} else {
		panic(util.Error("no virtualbox global system property %s", key))
	}
}
