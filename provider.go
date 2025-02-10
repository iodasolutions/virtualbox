package virtualbox

import (
	"context"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/util"
)

type Provider struct {
}

func (pv Provider) Up() ([]*provider.InstanceInfo, *cmd.XbeeError) {
	ctx := context.Background()
	vms := VmsFrom(ctx)
	if err := EnsureXbeenetExist(ctx); err != nil {
		return nil, err
	}
	downOrNotExisting, other := vms.NotExistingOrDown()
	for _, vm := range other {
		log2.Warnf("host %s is in state %s", vm.HostName, vm.info.State())
	}
	if err := downOrNotExisting.Up(ctx); err != nil {
		return nil, err
	}
	return pv.InstanceInfos()
}

func (pv Provider) Delete() *cmd.XbeeError {
	ctx := context.Background()
	vms := VmsFrom(ctx)
	existing, notExisting := vms.Existing()
	if len(notExisting) > 0 {
		log2.Infof("instances %v already do not exist", notExisting.Names())
	}
	for _, vm := range existing {
		if err := vm.Destroy(ctx); err != nil {
			return err
		}
	}
	return EnsureXbeenetDeleted(ctx)
}

func (pv Provider) InstanceInfos() ([]*provider.InstanceInfo, *cmd.XbeeError) {
	ctx := context.Background()
	vms := VmsFrom(ctx)
	var result []*provider.InstanceInfo
	for _, vm := range vms {
		info, err := vm.InstanceInfo(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}

func (pv Provider) Image() *cmd.XbeeError {
	ctx := context.Background()
	vms := VmsFrom(ctx)
	var list []util.Executor
	for _, vm := range vms {
		list = append(list, vm.ExportToVmdk)
	}
	err := util.Execute(ctx, list...)
	if err != nil {
		return err
	}
	log2.Infof("Export SUCCESSFULL")
	return nil
}

func (pv Provider) Down() *cmd.XbeeError {
	ctx := context.Background()
	vms := VmsFrom(ctx)
	down, _ := vms.Down()
	for _, vm := range down {
		if err := vm.AfterDown(ctx); err != nil {
			return err
		}
	}
	return nil
}
