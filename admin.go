package virtualbox

import (
	"context"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/provider"
)

type Admin struct {
}

func (a Admin) DestroyVolumes(names []string) *cmd.XbeeError {
	log2.Infof("destroy volumes %v ...", names)
	volumes := provider.VolumesFromEnvironment(names)
	ctx := context.Background()
	for _, vol := range volumes {
		boxVol := VboxVolumeFrom(vol)
		if err := boxVol.Delete(ctx); err != nil {
			return err
		}
	}
	return nil
}
