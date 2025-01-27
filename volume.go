package virtualbox

import (
	"context"
	"fmt"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/newfs"
	"github.com/iodasolutions/xbee-common/provider"
	"strings"
)

type VboxVolume struct {
	Name     string
	Size     int
	Location newfs.Folder `json:"location,omitempty"`
	Format   string
	//computed
	Device string
}

func VboxVolumeFrom(vol *provider.XbeeVolume) *VboxVolume {
	result := &VboxVolume{}
	if result.Location.String() == "" {
		result.Location = newfs.Volumes()
	} else {
		result.Location = newfs.NewFolder(newfs.CWD().ResolvePath(result.Location.String()))
	}
	if result.Format == "" {
		result.Format = "VDI"
	}
	result.Name = vol.Name
	result.Size = vol.Size * 1024 //virtualbox expect size in Mb
	return result
}

func (v *VboxVolume) File() newfs.File {
	return v.Location.ChildFile(fmt.Sprintf("%s.%s", v.Name, strings.ToLower(v.Format)))
}

func (v *VboxVolume) create(ctx context.Context) *cmd.XbeeError {
	log2.Infof("Create medium %s on host", v.File())
	err := VboxFrom("").CreateMedium(ctx, v.File(), v.Size, v.Format)
	return err
}
func (v *VboxVolume) Delete(ctx context.Context) *cmd.XbeeError {
	log2.Infof("Delete medium %s on host", v.File())
	return VboxFrom("").RemoveMedium(ctx, v.File())
}

func (v *VboxVolume) EnsureHostVolumeAttached(ctx context.Context, vm *Vm) *cmd.XbeeError {
	attachedVolumes := vm.info.AttachedVolumes()
	volumePort, ok := attachedVolumes[v.File().String()]
	if !ok {
		maxPort := 0
		for _, port := range attachedVolumes {
			if port > maxPort {
				maxPort = port
			}
		}
		volumePort = maxPort + 1
		log2.Infof("Attaching volume %s to vm %s", v.Name, vm.HostName)
		if err := vm.Vbox().AttachMedium(ctx, v.File(), "hdd", volumePort); err != nil {
			return err
		}
	}
	toto := "abcde"
	v.Device = fmt.Sprintf("/dev/sd%s1", string(toto[volumePort]))
	return nil
}

func (v *VboxVolume) EnsureHostVolumeDetached(ctx context.Context, vm *Vm) error {
	attachedVolumes := vm.info.AttachedVolumes()
	volumePort, ok := attachedVolumes[v.File().String()]
	if ok {
		log2.Infof("Detach volume %s from vm %s", v.Name, vm.HostName)
		return vm.Vbox().DetachMedium(ctx, volumePort)
	}
	return nil
}

type Hdd struct { // information from vboxmanage
	Location string
	Format   string
	Capacity string
}
