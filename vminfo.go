package virtualbox

import (
	"context"
	"fmt"
	"github.com/iodasolutions/xbee-common/constants"
	"strconv"
	"strings"
)

type vminfo struct {
	infos map[string]string //raw

	natRules map[string]string
}

// VmInfoFor returns a map containing output from command:
// vboxmanage showvminfo <Name> --machinereadable
// surrounding quotation marks are removed, only data in form key=value are kept.
func VmInfoFor(ctx context.Context, vmName string) *vminfo {
	out, err := VboxFrom(vmName).showVmInfo(ctx)
	if err != nil {
		aMap := make(map[string]string)
		aMap["VMState"] = "UNDEFINED" //No VM
		return &vminfo{
			infos: aMap,
		}
	} else {
		parser := &Parser{content: out}
		return &vminfo{
			infos: parser.asMap(),
		}
	}
}

func (info *vminfo) State() string {
	aState := info.infos["VMState"]
	if aState == "running" {
		return constants.State.Up
	} else if (aState == "poweroff") || (aState == "aborted") {
		return constants.State.Down
	}
	return constants.State.NotExisting
}

func (info *vminfo) SharedFolders() map[string]string {
	var key, value string
	result := map[string]string{}
	for k := range info.infos {
		if strings.HasPrefix(k, "SharedFolderNameMachineMapping") {
			key = info.infos[k]
		}
		if strings.HasPrefix(k, "SharedFolderPathMachineMapping") {
			value = info.infos[k]
		}
		if key != "" && value != "" {
			result[key] = value
			key = ""
			value = ""
		}
	}
	return result
}

func (info *vminfo) HostPorts() string {
	keys := info.allKeyStartingWith("Forwarding(")
	for _, key := range keys {
		value := info.infos[key]
		portS := info.portHelper("22", value)
		if portS != "" {
			return portS
		}
	}
	return ""
}

func extraValueFrom(line string) string {
	//Value: 192.168.101.3
	trimmed := strings.TrimSpace(line)
	return strings.TrimPrefix(trimmed, "Value: ")
}

func (info *vminfo) portHelper(guestPort string, s string) string {
	if strings.HasSuffix(s, fmt.Sprintf(",%s", guestPort)) {
		splitted := strings.Split(s, ",")
		portS := splitted[3]
		return portS
	}
	return ""
}

func (info *vminfo) NATRules() map[string]string { // map rule name to exposed host port
	if info.natRules == nil {
		info.natRules = make(map[string]string)
		keys := info.allKeyStartingWith("Forwarding(")
		for _, key := range keys {
			value := info.infos[key]
			splitted := strings.Split(value, ",")
			info.natRules[splitted[0]] = splitted[3]
		}
	}
	return info.natRules
}

func (info *vminfo) allKeyStartingWith(name string) (result []string) {
	for key := range info.infos {
		if strings.HasPrefix(key, name) {
			result = append(result, key)
		}
	}
	return
}
func (info *vminfo) NextAvailableSATAControllerPort() int {
	attachedVolumes := info.AttachedVolumes()
	maxPort := 0
	for _, port := range attachedVolumes {
		if port > maxPort {
			maxPort = port
		}
	}
	return maxPort + 1
}

func (info *vminfo) AttachedVolumes() map[string]int { // map filename to port number in sata controller
	result := make(map[string]int)
	for key := range info.infos {
		if strings.HasPrefix(key, "SATA-") && !strings.HasPrefix(key, "SATA-ImageUUID") {
			key1 := key[strings.Index(key, "-")+1:]
			indexS := key1[:strings.Index(key1, "-")]
			index, err := strconv.Atoi(indexS)
			if err != nil {
				panic(fmt.Errorf("unexpected error when extracting port number from %s", key))
			}
			if index != 0 {
				value := info.infos[key]
				if value != "none" {
					result[value] = index
				}
			}
		}
	}
	return result
}

func (info *vminfo) hostGuestPort(portMapping string) (hostPort string, guestPort string) {
	index := strings.Index(portMapping, ":")
	if index != -1 {
		hostPort = portMapping[:index]
		hostPort = strings.TrimSpace(hostPort)
		guestPort = portMapping[index+1:]
		guestPort = strings.TrimSpace(guestPort)
	} else {
		guestPort = portMapping
		guestPort = strings.TrimSpace(guestPort)
		NextPort.Lock()
		defer NextPort.Unlock()
		hostPort = NextPort.NextFreePort(guestPort)
	}
	return
}

func (info *vminfo) natKeyFor(port string) string {
	portSource := port
	index := strings.Index(port, ":")
	if index != -1 {
		portSource = port[index+1:]
	}
	return fmt.Sprintf("PRODUCT-%s", portSource)
}

func (info *vminfo) MacAddress(n int) string {
	key := fmt.Sprintf("macaddress%d", n)
	if value, ok := info.infos[key]; ok {
		return fmt.Sprintf("%s:%s:%s:%s:%s:%s", value[0:2], value[2:4], value[4:6], value[6:8], value[8:10], value[10:12])
	}
	return ""
}

func (info *vminfo) IsSeedAttached() bool {
	value, ok := info.infos["IDE-0-0"]
	return ok && value != "none"
}
func (info *vminfo) IsGuestAdditionAttached() bool {
	value, ok := info.infos["IDE-0-1"]
	return ok && value != "none"
}
