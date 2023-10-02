package virtualbox

import (
	"encoding/json"
	"fmt"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/newfs"
	"github.com/iodasolutions/xbee-common/util"
)

type Model struct {
	Cpus   int    `json:"cpus,omitempty"`
	Memory int    `json:"memory,omitempty"`
	Disk   string `json:"disk,omitempty"`
}

func fromMap(aMap map[string]interface{}) (*Model, error) {
	var result Model
	data, err := util.NewJsonIO(aMap).SaveAsBytes()
	if err != nil {
		return nil, fmt.Errorf("unexpected when encoding to json : %v", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unexpected when decoding json to AWS model : %v", err)
	}
	return &result, nil
}

func (m *Model) File() newfs.File {
	return newfs.XbeeIntern().CachedFileForUrl(m.Disk)
}

func (m *Model) ToBytes() []byte {
	if data, err := json.Marshal(m); err != nil {
		panic(cmd.Error("unable to serialize provider model : %v", err))
	} else {
		return data
	}
}
