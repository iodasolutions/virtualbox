package virtualbox

import (
	"context"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/constants"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/util"
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

func (vms Vms) NotExisting() (result Vms) {
	for _, vm := range vms {
		if vm.NotExisting() {
			result = append(result, vm)
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

func (vms Vms) Up(ctx context.Context) (err *cmd.XbeeError) {
	notExistinOrDown, _ := vms.NotExistingOrDown()
	for _, vm := range notExistinOrDown {
		if vm.NotExisting() {
			vm.InitiallyNotExisting = true
			if err := vm.prepareForCreation(ctx); err != nil {
				return err
			}
		} else {
			vm.InitiallyDown = true
		}
		err = vm.Start(ctx)
		if err != nil {
			return
		}
	}
	var list []util.Executor
	for _, vm := range notExistinOrDown {
		list = append(list, vm.waitSSH)
	}
	if err = util.Execute(ctx, list...); err != nil {
		return
	}
	for _, vm := range notExistinOrDown {
		if vm.InitiallyNotExisting {
			if err = vm.waitUntilCloudInitFinished(); err != nil {
				return
			}
			if !vm.Host.EffectiveDisk().Exists() {
				if err = vm.conn.RunScript(GuestAdditionScript()); err != nil {
					return
				}
				log2.Infof("%s : guest addition installed", vm.HostName)
				if err := vm.conn.RunCommandQuiet("sudo cloud-init clean"); err != nil {
					return err
				}
			}
		}
		if err = vm.conn.RunCommand("sudo mkdir -p /root/.xbee/cache-artefacts && sudo mount -t vboxsf xbee /root/.xbee/cache-artefacts"); err != nil {
			return
		}
		log2.Infof("shared folder xbee mounted")
		if vm.InitiallyNotExisting {
			if err = vm.conn.RunCommand("sudo cp /root/.xbee/cache-artefacts/s3.eu-west-3.amazonaws.com/xbee.repository.public/linux_amd64/xbee /usr/bin"); err != nil {
				return
			}
		}
	}
	return nil
}
