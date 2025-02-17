package virtualbox

import (
	"context"
	"fmt"
	"github.com/iodasolutions/virtualbox/properties"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/constants"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/newfs"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/ssh2"
	"github.com/iodasolutions/xbee-common/util"
	"os"
	"strconv"
	"strings"
	"time"
)

type Vm struct {
	HostName string
	User     string
	Ports    []string
	sshPort  string // set at runtime
	//cache
	volumes map[string]*VboxVolume
	Host    *Host

	//lazzily loaded
	originDisk           newfs.File //computed only when creating a new VM.
	info                 *vminfo
	InitiallyNotExisting bool
	InitiallyDown        bool
	guestAddition        *newfs.File
	conn                 *ssh2.SSHClient // set by waitSSH
}

func fromHost(ctx context.Context, h *provider.XbeeHost) (*Vm, error) {
	theH, err := NewHost(h)
	if err != nil {
		return nil, err
	}
	vm := &Vm{
		HostName: h.Name,
		User:     h.User,
		Host:     theH,
		Ports:    h.Ports,
	}
	vm.info = VmInfoFor(ctx, vm.Name())
	return vm, nil
}

func (vm *Vm) Name() string {
	return fmt.Sprintf("%s_%s", vm.HostName, provider.EnvId())
}

func (vm *Vm) Folder() newfs.Folder {
	return properties.VmFolder(vm.Name())
}

func (vm *Vm) SSHPort() string {
	if vm.sshPort == "" {
		vm.sshPort = vm.info.HostPorts()
	}
	return vm.sshPort
}

func (vm *Vm) Vbox() *Vbox {
	return VboxFrom(vm.Name())
}

func (vm *Vm) Destroy(ctx context.Context) *cmd.XbeeError {
	state := vm.info.State()
	if state == constants.State.Up {
		vb := vm.Vbox()
		if _, err := vb.execute(ctx, "controlvm", vb.name, "poweroff"); err != nil {
			return err
		}
	}
	if err := vm.AfterDown(ctx); err != nil {
		return err
	}
	if err := vm.EnsureVolumesDetached(ctx); err != nil {
		return err
	}
	if err := vm.Vbox().Unregister(ctx); err != nil {
		return err
	}
	if err := vm.Vbox().RemoveMedium(ctx, vm.VirtualDisk()); err != nil {
		return err
	}
	if err := vm.Folder().Delete(); err != nil {
		return err
	}
	log2.Infof("%s : vm removed", vm.HostName)
	return nil
}

func extractVmdk(aPath newfs.File) (newfs.File, *cmd.XbeeError) {
	if strings.HasSuffix(aPath.String(), ".vmdk") {
		return aPath, nil
	} else {
		name := aPath.Base()
		index := strings.LastIndex(name, ".")
		if index != -1 {
			result := aPath.Dir().ChildFile(name[:index] + ".vmdk")
			if result.Exists() {
				return result, nil
			}
		}
		if strings.HasSuffix(aPath.String(), ".box") {
			targetPath := strings.TrimSuffix(aPath.String(), ".box") + ".vmdk"
			tarPath := strings.TrimSuffix(aPath.String(), ".box") + ".tar"
			if err := os.Rename(aPath.String(), tarPath); err != nil {
				return newfs.NewFile(""), cmd.Error("cannot rename %s to %s: %v", aPath, tarPath, err)
			}
			defer func() {
				if err := os.Rename(tarPath, aPath.String()); err != nil {
					panic(fmt.Errorf("cannot rename %s to %s", tarPath, aPath))
				}
			}()
			if f, err := newfs.NewFile(tarPath).DecompressTar(); err != nil {
				return f, cmd.Error("cannot extract %s: %v", tarPath, err)
			}
			children := newfs.NewFile(tarPath).Dir().ChildrenFilesEndingWith(".vmdk")
			if len(children) == 1 {
				if targetPath != children[0].String() {
					if err := os.Rename(children[0].String(), targetPath); err != nil {
						return newfs.NewFile(""), cmd.Error("cannot rename %s to %s: %v", children[0], targetPath, err)
					}
				}
				return newfs.NewFile(targetPath), nil
			} else {
				return newfs.NewFile(""), cmd.Error("file %s is a tar file, but it contains no component with extension vmdk", aPath)
			}
		} else {
			return newfs.NewFile(""), cmd.Error("unrecognized extension in file : %s", aPath)
		}
	}
}

func (vm *Vm) VirtualDisk() newfs.File {
	return vm.Folder().ChildFile("xbee-system.vmdk")
}
func (vm *Vm) computeOriginVmdk(ctx context.Context) (err *cmd.XbeeError) {
	originDisk := vm.Host.OriginDisk()
	if !originDisk.Exists() {
		if originDisk, err = DownloadIfNotCached(ctx, vm.Host.Specification.Disk); err != nil {
			return
		}
		if originDisk, err = extractVmdk(originDisk); err != nil {
			return
		}
		if vm.guestAddition, err = EnsureGuestAdditions(ctx); err != nil {
			return
		}
	}
	vm.originDisk = originDisk
	return
}

func (vm *Vm) prepareForCreation(ctx context.Context) (err *cmd.XbeeError) {
	if err = vm.computeOriginVmdk(ctx); err != nil {
		return
	}
	//	vmdkOrigin.CopyToPath(vm.VirtualDisk())
	vb := vm.Vbox()
	log2.Infof("Create disk %s", vm.VirtualDisk())
	if err = vb.cloneMedium(ctx, vm.originDisk, vm.VirtualDisk()); err != nil {
		return err
	}
	log2.Infof("%s : Host does not exist, first create it", vm.HostName)
	if _, err = vb.execute(ctx, "createvm", "--name", vm.Name(), "--ostype", vm.Host.Specification.OsType, "--register"); err != nil {
		return
	}
	if _, err = vb.execute(ctx, "storagectl", vm.Name(), "--name", "SATA", "--add", "sata"); err != nil {
		return err
	}
	if _, err = vb.execute(ctx, "storagectl", vm.Name(), "--name", "IDE", "--add", "IDE"); err != nil {
		return err
	}
	if err = vb.Modify(ctx, "--boot1", "disk"); err != nil {
		return
	}
	iso := IsoFor(vm)
	if err = iso.CreateAndAttach(ctx); err != nil {
		return
	}
	if vm.guestAddition != nil {
		if err = vb.attacheDvdStorage(ctx, *vm.guestAddition, "1"); err != nil {
			return
		}
	}
	if err = vb.attachHddStorage(ctx, vm.VirtualDisk(), "0"); err != nil {
		return
	}
	if err = vm.configureNic(ctx); err != nil {
		return
	}
	return
}

func (vm *Vm) waitUntilCloudInitFinished() *cmd.XbeeError {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Minute)
	ff := func(ctx context.Context) *cmd.XbeeError {
		for {
			out, _ := vm.conn.RunCommandToOut("if [ -f /var/lib/cloud/instance/boot-finished ]; then echo 1; else echo 0; fi")
			out = strings.TrimSpace(out)
			if out == "1" {
				return nil
			}
			time.Sleep(time.Second)
		}
	}
	return util.Execute(ctx, ff)
}

func (vm *Vm) waitDown(ctx context.Context) *cmd.XbeeError {
	ff := func(ctx context.Context) *cmd.XbeeError {
		for {
			vm.info = VmInfoFor(ctx, vm.Name())
			if vm.info.State() == constants.State.Down {
				return nil
			}
			time.Sleep(time.Second)
		}
	}
	return util.Execute(ctx, ff)
}

func (vm *Vm) waitSSH(ctx context.Context) *cmd.XbeeError {
	log2.Infof("%s : wait until SSH open...", vm.HostName)
	ff := func(_ context.Context) *cmd.XbeeError {
		for {
			var err *cmd.XbeeError
			vm.conn, err = ssh2.Connect("127.0.0.1", vm.sshPort, vm.User)
			if err == nil {
				return nil
			}
			log2.Infof("%s : SSH connection to 127.0.0.1:%s still not opened for user %s", vm.HostName, vm.sshPort, vm.User)
			time.Sleep(time.Second)
		}
	}
	if err := util.Execute(ctx, ff); err != nil {
		return err
	}
	log2.Infof("%s : SSH connexion OK", vm.HostName)
	return nil
}

func (vm *Vm) ExportToVmdk(ctx context.Context) (err *cmd.XbeeError) {
	vm.conn, err = ssh2.Connect("127.0.0.1", vm.SSHPort(), vm.User)
	if err != nil {
		return
	}
	if err = vm.conn.RunCommandQuiet("sudo cloud-init clean"); err != nil {
		return
	}
	//if err = vm.conn.RunCommandQuiet("sudo rm /etc/machine-id"); err != nil {
	//	return
	//}
	if err = vm.conn.RunCommandQuiet("sudo shutdown -P now"); err != nil {
		return
	}
	if err = vm.AfterDown(ctx); err != nil {
		return
	}
	var vmdkPath *newfs.File
	if vmdkPath, err = vm.Vbox().export(ctx); err != nil {
		return
	}
	defer vmdkPath.Dir().Delete()
	log2.Infof("Export vm %s to [%s]...", vm.HostName, vmdkPath)
	targetDisk := vm.Host.TargetDiskForImage()
	targetDisk.Dir().EnsureExists()
	if err2 := os.Rename(vmdkPath.String(), targetDisk.String()); err2 != nil {
		err = cmd.Error("failed to move %s to %s", vmdkPath, targetDisk.String())
	}
	return
}

func (vm *Vm) configureNic(ctx context.Context) *cmd.XbeeError {
	vb := vm.Vbox()
	if err := vb.Modify(ctx, "--nic1", "nat"); err != nil {
		return err
	}
	if err := vb.Modify(ctx, "--nic2", "intnet"); err != nil {
		return err
	}
	return vb.Modify(ctx, "--intnet2", DefaultNet())
}

func (vm *Vm) Start(ctx context.Context) (err *cmd.XbeeError) {
	if err = vm.DeleteNATRules(ctx); err != nil {
		return
	}
	if err = vm.EnsureXbeeSharedFolder(ctx); err != nil {
		return
	}
	if err = vm.EnsureHostVolumesExistAndAttached(ctx); err != nil {
		return
	}
	if err = vm.configureSharedPorts(ctx); err != nil {
		return
	}
	if err = vm.configureMemoryAndCpus(ctx); err != nil {
		return
	}
	log2.Infof("%s : Start vm...", vm.HostName)
	if err = vm.Vbox().Start(ctx); err != nil {
		return
	}
	vm.info = VmInfoFor(ctx, vm.Name())
	return
}

func (vm *Vm) EnsureXbeeSharedFolder(ctx context.Context) *cmd.XbeeError {
	if _, ok := vm.info.SharedFolders()["xbee"]; !ok {

		if err := vm.Vbox().AddSharedFolder(ctx, newfs.CacheArtefacts().String(), "xbee"); err != nil {
			return err
		}
	}
	return nil
}

func (vm *Vm) EnsureHostVolumesExistAndAttached(ctx context.Context) *cmd.XbeeError {
	for volName, volume := range vm.volumes {
		if strings.HasPrefix(volName, "/") {
			if err := vm.addSharedFolder(ctx, volName); err != nil {
				return err
			}
		} else {
			if !volume.File().Exists() {
				if err2 := volume.create(ctx); err2 != nil {
					return err2
				}
			}
			if err := volume.EnsureHostVolumeAttached(ctx, vm); err != nil {
				return err
			}
		}
	}
	return nil
}

func (vm *Vm) addSharedFolder(ctx context.Context, volumeName string) *cmd.XbeeError {
	if volumeName != "" && strings.HasPrefix(volumeName, "/") {
		hostFolder := newfs.NewFolder(volumeName)
		hostFolder.EnsureExists()
		log2.Infof("Configure in virtualbox shared folder \n\t(guest) %s\n\t(host) %s", hostFolder.Path.Hash(), volumeName)
		return vm.Vbox().AddSharedFolder(ctx, hostFolder.String(), hostFolder.Path.Hash())
	}
	return nil
}

// configureMemoryAndCpus is used before starting a new or existing VM.
func (vm *Vm) configureMemoryAndCpus(ctx context.Context) *cmd.XbeeError {
	return vm.Vbox().Modify(ctx, "--memory", strconv.Itoa(vm.Host.Specification.Memory), "--cpus", strconv.Itoa(vm.Host.Specification.Cpus))
}

// configureSharedPorts is used before starting a new or existing VM.
func (vm *Vm) configureSharedPorts(ctx context.Context) (err *cmd.XbeeError) {
	if err = vm.assignSSHPortFromHost(ctx); err != nil {
		return
	}
	return vm.ExposePorts(ctx, vm.Ports)
}

func (vm *Vm) assignSSHPortFromHost(ctx context.Context) *cmd.XbeeError {
	sshPortS := vm.sshPort
	if sshPortS == "" {
		sshPortS = "0"
	}
	sshPort, _ := strconv.Atoi(sshPortS) //should not occur at this stage
	if sshPort < 1024 {
		sshPort = 2200
	}
	NextPort.Lock()
	defer NextPort.Unlock()
	portS := NextPort.NextFreePort(strconv.Itoa(sshPort))
	if err := vm.Vbox().Modify(ctx, "--natpf1", fmt.Sprintf("ssh,tcp,127.0.0.1,%s,,22", portS)); err != nil {
		return err
	}
	vm.sshPort = portS
	return nil
}

func (vm *Vm) EnsureVolumesDetached(ctx context.Context) *cmd.XbeeError {
	for name := range vm.volumes {
		if !strings.HasPrefix(name, "/") {
			attachedVolumes := vm.info.AttachedVolumes()
			for _, port := range attachedVolumes {
				log2.Infof("Detach volume %s from vm %s", name, vm.HostName)
				if err := vm.Vbox().DetachMedium(ctx, port); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (vm *Vm) AfterDown(ctx context.Context) *cmd.XbeeError {
	if err := vm.waitDown(ctx); err != nil {
		return err
	}
	if vm.info.IsSeedAttached() {
		iso := IsoFor(vm)
		if err := iso.DetachAndDelete(ctx); err != nil {
			return err
		}
	}
	if vm.info.IsGuestAdditionAttached() {
		if err := DetachGuestAdditions(ctx, vm.Name()); err != nil {
			return err
		}
	}
	return vm.DeleteNATRules(ctx)
}

func (vm *Vm) DeleteNATRules(ctx context.Context) *cmd.XbeeError {
	for name, port := range vm.info.NATRules() {
		if err := vm.Vbox().DeleteNATRule(ctx, name); err != nil {
			return err
		}
		log2.Debugf("Deleted exposed port %s", port)
	}
	return nil
}

func (vm *Vm) ExposePorts(ctx context.Context, ports []string) *cmd.XbeeError {
	for _, port := range ports {
		natKey := vm.info.natKeyFor(port)
		if _, ok := vm.info.NATRules()[natKey]; ok {
			log2.Warnf("port %s is already exposed for host %s. Skip action", strings.TrimLeft(natKey, "PRODUCT-"), vm.HostName)
		} else {
			hostPort, guestPort := vm.info.hostGuestPort(port)
			if err := vm.Vbox().AddNATRule(ctx, natKey, hostPort, guestPort); err != nil {
				return err
			}
		}
	}
	return nil
}

func (vm *Vm) NotExistingOrDown() bool {
	state := vm.info.State()
	return state == constants.State.Down || state == constants.State.NotExisting
}

func (vm *Vm) NotExisting() bool {
	state := vm.info.State()
	return state == constants.State.NotExisting
}

func (vm *Vm) InstanceInfo(ctx context.Context) (*provider.InstanceInfo, *cmd.XbeeError) {
	var ip string
	if vm.info.State() == constants.State.Up {
		out, err := vm.Vbox().GetProperty(ctx, "/VirtualBox/GuestInfo/Net/1/V4/IP")
		if err != nil {
			return nil, err
		}
		ip = extraValueFrom(out)
	}
	return &provider.InstanceInfo{
		Name:          vm.HostName,
		State:         vm.info.State(),
		ExternalIp:    "127.0.0.1",
		SSHPort:       vm.SSHPort(),
		Ip:            ip,
		User:          vm.User,
		PackIdExist:   vm.Host.PackDisk().Exists(),
		SystemIdExist: vm.Host.SystemDisk().Exists(),
	}, nil
}

// No value set!
func (vm *Vm) InternalIp(ctx context.Context) (string, error) {
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
			out, err := vm.Vbox().GetProperty(ctx, "/VirtualBox/GuestInfo/Net/1/V4/IP")
			if err != nil {
				return "", err
			}
			ip := extraValueFrom(out)
			if ip != "No value set!" {
				return ip, nil
			}
		}
	}
}
