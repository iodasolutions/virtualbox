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
	//var l []*Vm
	//for _, vm := range vms {
	//	if !vm.InitiallyNotExisting && vm.info.State() == constants.State.Up {
	//		l = append(l, vm)
	//	}
	//}
	//ips := map[int]bool{}
	//if len(l) > 0 {
	//	var wg sync.WaitGroup
	//	wg.Add(len(l))
	//	var aLock sync.Mutex
	//	for _, vm := range l {
	//		go func(vm *Vm) {
	//			defer wg.Done()
	//			ip, err := vm.InternalIp(ctx)
	//			if err == nil {
	//				aLock.Lock()
	//				defer aLock.Unlock()
	//				s := strings.Split(ip, ".")[3]
	//				anInt, _ := strconv.Atoi(s)
	//				ips[anInt] = true
	//			}
	//		}(vm)
	//	}
	//	wg.Wait()
	//}
	//var l2 []*Vm
	//for _, vm := range vms {
	//	if vm.InitiallyNotExisting {
	//		l2 = append(l2, vm)
	//	}
	//}
	//if len(l2) > 0 {
	//	var wg sync.WaitGroup
	//	wg.Add(len(l2))
	//	var counter int
	//	for _, vm := range l2 {
	//		for {
	//			counter++
	//			if counter == 255 {
	//				return nil, cmd.Error("exceeded max number possible ip for network")
	//			}
	//			if _, ok := ips[counter]; !ok {
	//				break
	//			}
	//		}
	//		go func(vm *Vm, index int) {
	//			defer wg.Done()
	//			if err := vm.configureNetwork(ctx, index); err != nil {
	//				log2.Errorf("cannot configure network for %s", vm.HostName)
	//			}
	//		}(vm, counter)
	//	}
	//	wg.Wait()
	//}
	//
	//status := &provider.InitialStatus{
	//	NotExisting: map[string]*provider.InstanceInfo{},
	//	Down:        map[string]*provider.InstanceInfo{},
	//	Other:       map[string]*provider.InstanceInfo{},
	//	Up:          map[string]*provider.InstanceInfo{},
	//}
	//for _, vm := range vms {
	//	info, err := vm.InstanceInfo(ctx)
	//	if err != nil {
	//		return nil, err
	//	}
	//	if vm.InitiallyNotExisting {
	//		status.NotExisting[vm.HostName] = info
	//	} else if vm.InitiallyDown {
	//		status.Down[vm.HostName] = info
	//	} else {
	//		if info.State == constants.State.Up {
	//			status.Up[vm.HostName] = info
	//		} else {
	//			status.Other[vm.HostName] = info
	//		}
	//	}
	//}
	//
	//for _, info := range status.AllUp() {
	//
	//	log2.Infof("ip %s=%s", info.Name, info.Ip)
	//}
	//
	//return status, nil
	return pv.InstanceInfos()
}

func (pv Provider) Delete() *cmd.XbeeError {
	ctx := context.Background()
	vms := VmsFrom(ctx)
	existing, notExisting := vms.Existing()
	if len(notExisting) > 0 {
		log2.Infof("instances %v already do not exist", notExisting.Names())
	}
	var list []util.Executor
	for _, vm := range existing {
		list = append(list, vm.Destroy)
	}
	err := util.Execute(ctx, list...)
	if err != nil {
		return err
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
