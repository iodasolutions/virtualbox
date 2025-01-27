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

type Vms []*Vm

func VmsFrom(ctx context.Context) (result Vms) {
	for vm := range vmGenerator(ctx) {
		result = append(result, vm)
	}
	return
}

func (vms Vms) Names() (result []string) {
	for _, vm := range vms {
		result = append(result, vm.Name())
	}
	return
}
func (vms Vms) Existing() (result Vms, other Vms) {
	for _, vm := range vms {
		if vm.info.State() == constants.State.NotExisting {
			other = append(other, vm)
		} else {
			result = append(result, vm)
		}
	}
	return
}

func (vms Vms) NotExistingOrDown() (result Vms, other Vms) {
	for _, vm := range vms {
		if vm.NotExistingOrDown() {
			result = append(result, vm)
		} else {
			other = append(other, vm)
		}
	}
	return
}

func vmGenerator(ctx context.Context) <-chan *Vm {
	vols := map[string]*VboxVolume{}
	for _, vol := range provider.VolumesForEnv() {
		vols[vol.Name] = VboxVolumeFrom(vol)
	}
	var channels []<-chan *Vm
	for _, h := range provider.Hosts() {
		ch := make(chan *Vm)
		channels = append(channels, ch)
		go func(h *provider.XbeeHost) {
			defer close(ch)
			var vm *Vm
			vm, err := fromHost(ctx, h)
			if err != nil {
				log2.Errorf(err.Error())
			} else {
				vm.volumes = map[string]*VboxVolume{}
				for _, name := range h.Volumes {
					vm.volumes[name] = vols[name] // value can be nil
				}
				ch <- vm
			}
		}(h)
	}
	return util.Multiplex(ctx, channels...)
}

type Vm struct {
	HostName string
	User     string
	Ports    []string
	sshPort  string // set at runtime
	//cache
	volumes map[string]*VboxVolume
	Host    *Host

	//lazzily loaded
	info                 *vminfo
	InitiallyNotExisting bool
	InitiallyDown        bool

	conn *ssh2.SSHClient // set by waitSSH
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

func (vm *Vm) HaltInstance(ctx context.Context) *cmd.XbeeError {
	log2.Infof("Shutdown request...")
	vb := vm.Vbox()
	if err := vb.Stop(ctx); err != nil {
		return err
	}
	log2.Infof("...Shutdown succeeded")
	return vm.DeleteNATRules(ctx)
}

func (vm *Vm) Name() string {
	return fmt.Sprintf("%s_%s", vm.HostName, provider.EnvId())
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
		if err := vm.HaltInstance(ctx); err != nil {
			return err
		}
	}
	if err := vm.EnsureVolumesDetached(ctx); err != nil {
		return err
	}
	if err := vm.EnsureIsoDetached(ctx); err != nil {
		return err
	}
	log2.Infof("Remove vm from virtualbox...")
	if err := vm.Vbox().Unregister(ctx); err != nil {
		return err
	}
	properties.VmFolder(vm.Name()).Delete()
	log2.Infof("...vm removed")
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

func (vm *Vm) create(ctx context.Context) (err *cmd.XbeeError) {
	log2.Infof("%s : Host does not exist, first create it", vm.HostName)
	var originDisk newfs.File
	originDisk, err = DownloadIfNotCached(ctx, vm.Host.Specification.Disk)
	if err != nil {
		return
	}
	var vmdk newfs.File
	vmdk, err = extractVmdk(originDisk)
	if err != nil {
		return
	}
	vb := vm.Vbox()
	if err = vb.Create(ctx); err != nil {
		return
	}
	if err = vb.Modify(ctx, "--boot1", "disk"); err != nil {
		return
	}
	iso := IsoFor(vm)
	if err = iso.CreateAndAttach(ctx); err != nil {
		return
	}
	log2.Infof("cloud-init iso disk created and attached")
	if err = DownloadAndAttachGuestAdditions(ctx, vm.Name()); err != nil {
		return
	}
	if err = vb.attachHddStorage(ctx, vmdk, "0"); err != nil {
		return
	}
	log2.Infof("guest addition downloaded and  attached")
	if err = vm.configureNic(ctx); err != nil {
		return
	}
	err = vm.Start(ctx)
	if err != nil {
		return
	}
	if err = vm.waitUntilCloudInitFinished(); err != nil {
		return
	}
	log2.Infof("%s : cloud-init boot finished", vm.HostName)
	// PB with VB 7.0.8
	//if err = vm.conn.RunScript(GuestAdditionScript()); err != nil {
	//	return
	//}
	//log2.Infof("%s : guest addition installed", vm.HostName)
	//if needRestart(client) {
	//	log3.Infof("%s : need restart", vm.HostName)
	//	if err = vm.cleanCloudInitAndHalt(client); err != nil {
	//		return
	//	}
	//	ff := func(_ context.Context) error {
	//		for {
	//			time.Sleep(time.Second)
	//			vm.info = VmInfoFor(ctx, vm.Name())
	//			if vm.info.State() == constants.State.Down {
	//				return nil
	//			}
	//		}
	//	}
	//	if err = util.Execute(ctx, ff); err != nil {
	//		return
	//	}
	//	if err = vm.ExportToVmdk(ctx); err != nil {
	//		return
	//	}
	//	if err = vm.Destroy(ctx); err != nil {
	//		return
	//	}
	//	vm.info = VmInfoFor(ctx, vm.Name())
	//	return vm.create(ctx)
	//}
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
func (vm *Vm) needRestart() bool {
	out, _ := vm.conn.RunCommandToOut("if [ -f /var/xbee/restart ]; then echo 1; else echo 0; fi")
	out = strings.TrimSpace(out)
	return out == "1"
}

func (vm *Vm) cleanCloudInitAndHalt() error {
	if err := vm.conn.RunCommandQuiet(fmt.Sprintf("rm -rf /home/%s/.ssh", vm.User)); err != nil {
		return err
	}
	if err := vm.conn.RunCommandQuiet("rm -f /var/xbee/restart"); err != nil {
		return err
	}
	if err := vm.conn.RunCommandQuiet("cloud-init clean"); err != nil {
		return err
	}
	log2.Infof("halt host %s", vm.HostName)
	if err := vm.conn.RunCommandQuiet("shutdown -h now"); err != nil {
		log2.Infof("Lost connection to %s", vm.HostName)
	}
	return nil
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
			time.Sleep(time.Second)
		}
	}
	if err := util.Execute(ctx, ff); err != nil {
		return err
	}
	log2.Infof("%s : SSH connexion OK", vm.HostName)
	return nil
}

func (vm *Vm) ExportToVmdk(ctx context.Context) error {
	tmpDir := newfs.TmpDir().RandomChildFolder()
	tmpDir.EnsureExists()
	defer tmpDir.Delete()
	ovfPath := tmpDir.ChildFile("image.ovf")
	log2.Infof("Export vm %s to [%s]...", vm.HostName, ovfPath)
	if err := vm.Vbox().Export(ctx, ovfPath); err != nil {
		return err
	}
	vmdks := tmpDir.ChildrenFilesEndingWith(".vmdk")
	if len(vmdks) != 1 {
		panic(cmd.Error("expects one vmdk file in folder %s, actual is %d", tmpDir, len(vmdks)))
	}
	vmdk := vmdks[0]
	vmdk.CopyToPath(vm.Host.Specification.File())
	log2.Infof("Export SUCCESSFULL")
	return nil
}

func (vm *Vm) configureNic(ctx context.Context) *cmd.XbeeError {
	subnet, err := EnsureXbeenetExist(ctx)
	if err != nil {
		return err
	}
	vb := vm.Vbox()
	if err := vb.Modify(ctx, "--nic1", "nat"); err != nil {
		return err
	}
	if err := vb.Modify(ctx, "--nic2", "intnet"); err != nil {
		return err
	}
	return vb.Modify(ctx, "--intnet2", subnet)
}

func (vm *Vm) Start(ctx context.Context) *cmd.XbeeError {
	if err := vm.ConfigureBeforeStart(ctx); err != nil {
		return err
	}
	log2.Infof("Start vm...")
	if err := vm.Vbox().Start(ctx); err != nil {
		return err
	}
	vm.info = VmInfoFor(ctx, vm.Name())
	return vm.waitSSH(ctx)
}

func (vm *Vm) Up(ctx context.Context) (err *cmd.XbeeError) {
	if vm.info.State() == constants.State.NotExisting {
		vm.InitiallyNotExisting = true
		err = vm.create(ctx)
		if err != nil {
			return
		}
	} else {
		vm.InitiallyDown = true
		err = vm.Start(ctx)
		if err != nil {
			return
		}
	}
	//defer func() {
	//	err = util.CloseWithError(client.Close, err)
	//}()
	if err = vm.conn.RunCommand("sudo mkdir -p /root/.xbee/cache-artefacts && sudo mount -t vboxsf xbee /root/.xbee/cache-artefacts"); err != nil {
		return
	}
	log2.Infof("shared folder xbee mounted")
	if vm.InitiallyNotExisting {
		if err = vm.conn.RunCommand("sudo cp /root/.xbee/cache-artefacts/s3.eu-west-3.amazonaws.com/xbee.repository.public/linux_amd64/xbee /usr/bin"); err != nil {
			return
		}
	}
	return
}

func (vm *Vm) ConfigureBeforeStart(ctx context.Context) *cmd.XbeeError {
	if err := vm.DeleteNATRules(ctx); err != nil {
		return err
	}
	if err := vm.EnsureXbeeSharedFolder(ctx); err != nil {
		return err
	}
	if err := vm.EnsureHostVolumesExistAndAttached(ctx); err != nil {
		return err
	}
	if err := vm.configureSharedPorts(ctx); err != nil {
		return err
	}
	return vm.configureMemoryAndCpus(ctx)
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
	log2.Infof("memory set to %d Mo", vm.Host.Specification.Memory)
	log2.Infof("cpus set to %d", vm.Host.Specification.Cpus)
	return vm.Vbox().Modify(ctx, "--memory", strconv.Itoa(vm.Host.Specification.Memory), "--cpus", strconv.Itoa(vm.Host.Specification.Cpus))
}

// configureSharedPorts is used before starting a new or existing VM.
func (vm *Vm) configureSharedPorts(ctx context.Context) (err *cmd.XbeeError) {
	if err = vm.assignSSHPortFromHost(ctx); err != nil {
		return
	}
	if err != nil {
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
	log2.Infof("Expose ssh port 22 (guest) to port %s (host)", portS)
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

func (vm *Vm) EnsureIsoDetached(ctx context.Context) *cmd.XbeeError {
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
	return nil
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
			log2.Infof("Expose port %s (guest) to port %s (host)", guestPort, hostPort)
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
		Name:       vm.HostName,
		State:      vm.info.State(),
		ExternalIp: "127.0.0.1",
		SSHPort:    vm.SSHPort(),
		Ip:         ip,
		User:       vm.User,
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
