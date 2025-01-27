package virtualbox

import (
	"bytes"
	"context"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/template"
)

var network = `#!/bin/bash
set -e
rm /etc/netplan/*
cat > /etc/netplan/01-netcfg.yaml <<EOF 
network:
  version: 2
  renderer: networkd
  ethernets:
    enp0s3:
      dhcp4: yes
    enp0s8:
      dhcp4: no
      addresses: [192.168.122.{{ .index }}/24]
EOF
netplan apply
`

func networkScript(index int) string {
	model := map[string]interface{}{
		"index": index,
	}
	w := &bytes.Buffer{}
	if err := template.OutputWithTemplate(network, w, model, nil); err != nil {
		panic(cmd.Error("failed to parse network template : %v", err))
	}
	return w.String()
}

func (vm *Vm) configureNetwork(ctx context.Context, index int) error {
	if err := vm.conn.RunScript(networkScript(index)); err != nil {
		return err
	}
	if _, err := vm.InternalIp(ctx); err != nil {
		return err
	}
	return nil
}
