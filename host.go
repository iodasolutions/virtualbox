package virtualbox

import (
	"encoding/json"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/newfs"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/util"
)

type VboxHostData struct {
	Cpus      int    `json:"cpus,omitempty"`
	Memory    int    `json:"memory,omitempty"`
	Disk      string `json:"disk,omitempty"`
	OsType    string `json:"ostype,omitempty"`
	Bootstrap string `json:"cloud-init,omitempty"`
}

func (m *VboxHostData) File() newfs.File {
	return newfs.CachedFileForUrl(m.Disk)
}

type Host struct {
	*provider.XbeeHost
	Specification *VboxHostData
}

func NewHost(host *provider.XbeeHost) (*Host, *cmd.XbeeError) {
	var result VboxHostData
	data, err := util.NewJsonIO(host.Provider).SaveAsBytes()
	if err != nil {
		panic(cmd.Error("unexpected error when serializing data provider: %v", err))
	}
	if err := json.Unmarshal(data, &result); err != nil {
		panic(cmd.Error("unexpected error when deserializing data provider : %v", err))
	}
	mapData := provider.SystemProviderDataFor(host.SystemHash)
	result.Disk = mapData["disk"].(string)
	result.OsType = mapData["ostype"].(string)
	result.Bootstrap = mapData["cloud-init"].(string)
	return &Host{XbeeHost: host, Specification: &result}, nil
}

func (h *Host) EffectiveDisk() newfs.File {
	return ExportFolder().ChildFile(h.EffectiveHash() + ".vmdk")
}
func (h *Host) SystemDisk() newfs.File {
	return ExportFolder().ChildFile(h.SystemHash + ".vmdk")
}
