package virtualbox

import (
	"context"
	"fmt"
	"github.com/iodasolutions/virtualbox/properties"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/newfs"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

var vboxLock = &sync.Mutex{}

type Vbox struct {
	name string
}

func VboxFrom(name string) *Vbox {
	return &Vbox{
		name: name,
	}
}

func (vbox *Vbox) execute(ctx context.Context, partialCommand ...string) (string, *cmd.XbeeError) {
	aCmd := exec.CommandContext(ctx, properties.VboxPath(), partialCommand...)
	out, err := aCmd.CombinedOutput()
	if err != nil {
		commandS := properties.VboxPath() + " " + strings.Join(partialCommand, " ")
		return "", cmd.Error("command %s failed : output is :\n%s", commandS, out)
	}
	return string(out), nil
}

func (vbox *Vbox) Start(ctx context.Context) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "startvm", vbox.name, "-type", "headless")
	return err
}

func (vbox *Vbox) Stop(ctx context.Context) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "controlvm", vbox.name, "poweroff")
	return err
}

func (vbox *Vbox) Unregister(ctx context.Context) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "unregistervm", vbox.name)
	return err
}

func (vbox *Vbox) showVmInfo(ctx context.Context) (string, *cmd.XbeeError) {
	return vbox.execute(ctx, "showvminfo", vbox.name, "--machinereadable")
}

func (vbox *Vbox) Create(ctx context.Context) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "createvm", "--name", vbox.name, "--register")
	if err != nil {
		return err
	}
	_, err = vbox.execute(ctx, "storagectl", vbox.name, "--name", "SATA", "--add", "sata")
	if err != nil {
		return err
	}
	_, err = vbox.execute(ctx, "storagectl", vbox.name, "--name", "IDE", "--add", "IDE")
	return err
}

func (vbox *Vbox) Modify(ctx context.Context, args ...string) *cmd.XbeeError {
	args = append([]string{"modifyvm", vbox.name}, args...)
	_, err := vbox.execute(ctx, args...)
	return err
}

func (vbox *Vbox) AddSharedFolder(ctx context.Context, hostPath string, mountName string) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "sharedfolder", "add", vbox.name, "--name", mountName, "--hostpath", hostPath)
	if err != nil {
		return err
	}
	_, err = vbox.execute(ctx, "setextradata", vbox.name, fmt.Sprintf("VBoxInternal2/SharedFoldersEnableSymlinksCreate/%s", mountName), "1")
	return err
}

func (vbox *Vbox) AddNATRule(ctx context.Context, key string, hostPort string, guestPort string) *cmd.XbeeError {
	return vbox.Modify(ctx, "--natpf1", fmt.Sprintf("%s,tcp,,%s,,%s", key, hostPort, guestPort))
}

func (vbox *Vbox) DeleteNATRule(ctx context.Context, key string) *cmd.XbeeError {
	return vbox.Modify(ctx, "--natpf1", "delete", key)
}

func (vbox *Vbox) CreateMedium(ctx context.Context, location newfs.File, size int, format string) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "createmedium", "disk",
		"--filename", location.String(),
		"--size", strconv.Itoa(size),
		"--format", format)
	return err
}
func (vbox *Vbox) RemoveMedium(ctx context.Context, location newfs.File) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "closemedium", "disk", location.String(), "--delete")
	return err
}

func (vbox *Vbox) AttachMedium(ctx context.Context, location newfs.File, theType string, port int) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "storageattach", vbox.name,
		"--type", theType,
		"--storagectl", "SATA",
		"--port", strconv.Itoa(port),
		"--device", "0",
		"--medium", location.String())
	return err
}

func (vbox *Vbox) DetachMedium(ctx context.Context, port int) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "storageattach", vbox.name,
		"--type", "hdd",
		"--storagectl", "SATA",
		"--port", strconv.Itoa(port),
		"--device", "0",
		"--medium", "none")
	return err
}

func (vbox *Vbox) Import(ctx context.Context, ovf newfs.File) *cmd.XbeeError {
	log2.Infof("Import OVF [%s] into virtualbox", ovf)
	_, err := vbox.execute(ctx, "import", ovf.String(), "--vsys", "0", "--vmname", vbox.name)
	return err
}

func (vbox *Vbox) Export(ctx context.Context, ovf newfs.File) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "export", vbox.name, "-o", ovf.String())
	return err
}

func (vbox *Vbox) GetProperty(ctx context.Context, name string) (string, *cmd.XbeeError) {
	return vbox.execute(ctx, "guestproperty", "get", vbox.name, name)
}
func (vbox *Vbox) listDhcpServers(ctx context.Context) (string, *cmd.XbeeError) {
	return vbox.execute(ctx, "list", "dhcpservers")
}
func (vbox *Vbox) removeDhcpServer(ctx context.Context, xbeenetName string) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "dhcpserver", "remove",
		"--netname", xbeenetName)
	return err
}
func (vbox *Vbox) attacheDvdStorage(ctx context.Context, f newfs.File, device string) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "storageattach", vbox.name,
		"--type", "dvddrive",
		"--storagectl", "IDE",
		"--port", "0",
		"--device", device,
		"--medium", f.String())
	return err
}
func (vbox *Vbox) attachHddStorage(ctx context.Context, f newfs.File, device string) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "storageattach", vbox.name,
		"--type", "hdd",
		"--storagectl", "SATA",
		"--port", "0",
		"--device", device,
		"--medium", f.String())
	return err
}

func (vbox *Vbox) detachDvdStorage(ctx context.Context, device string) *cmd.XbeeError {
	_, err := vbox.execute(ctx, "storageattach", vbox.name,
		"--type", "dvddrive",
		"--storagectl", "IDE",
		"--port", "0",
		"--device", device,
		"--medium", "none")
	return err
}
