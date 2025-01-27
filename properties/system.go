package properties

import (
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/exec2"
	"github.com/iodasolutions/xbee-common/newfs"
	"strings"
	"sync"
)

var system struct {
	properties map[string]string
	once       sync.Once
}

func initSystem() map[string]string {
	c := exec2.NewCommand(VboxPath(), "list", "systemproperties").Quiet().WithResult()
	if err := c.Run(nil); err != nil {
		panic(err)
	} else {
		out := c.Result()
		lines := strings.Split(out, "\n")
		result := make(map[string]string)
		for _, line := range lines {
			// Ignorer les lignes vides
			if strings.TrimSpace(line) == "" {
				continue
			}

			// Diviser chaque ligne en clé et valeur
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				result[key] = value
			}
		}
		return result
	}

}

func defaultMachineFolder() string {
	return fromSystem("Default machine folder")
}

func VmFolder(name string) newfs.Folder {
	return newfs.NewFolder(defaultMachineFolder()).ChildFolder(name)
}

func fromSystem(key string) string {
	system.once.Do(func() {
		system.properties = initSystem()
	})
	if value, ok := system.properties[key]; ok {
		return value
	} else {
		panic(cmd.Error("no virtualbox global system property %s", key))
	}
}
