package virtualbox

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/iodasolutions/virtualbox/properties"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/newfs"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/template"
	"github.com/kdomanski/iso9660"
	"strings"
)

var metadata = `instance-id: iid-abcdefg
local-hostname: {{ .name }}
`

var userdata = `#cloud-config
write_files:
  - encoding: b64
    content: {{ .authorized }}
    owner: root:root
    path: /run/authorized-data.sh
runcmd:
  - bash /run/authorized-data.sh

`

/*
  - bash /run/additions-data.sh
  - bash /run/agent-data.sh

*/

type Iso struct {
	vm *Vm
}

func IsoFor(vm *Vm) *Iso {
	return &Iso{
		vm: vm,
	}
}

func (iso *Iso) authorizedKeyScript() string {
	script := `#!/bin/bash
{{ .authorized }}
`
	model := map[string]interface{}{
		"authorized": provider.AuthorizedKeyScript(iso.vm.User),
	}
	w := &bytes.Buffer{}
	if err := template.OutputWithTemplate(script, w, model, nil); err != nil {
		panic(cmd.Error("failed to parse authorizedKeyScript template : %v", err))
	}
	return w.String()
}

func (iso *Iso) CreateAndAttach(ctx context.Context) *cmd.XbeeError {
	aMap := map[string]interface{}{
		"name":       iso.vm.HostName,
		"authorized": base64.StdEncoding.EncodeToString([]byte(iso.authorizedKeyScript())),
	}
	t1 := metadata
	if err := template.Output(&t1, aMap, nil); err != nil {
		return cmd.Error("cannot parse template metadata: %v", err)
	}
	t2 := userdata
	if err := template.Output(&t2, aMap, nil); err != nil {
		return cmd.Error("cannot parse template userdata: %v", err)
	}
	writer, err2 := iso9660.NewWriter()
	if err2 != nil {
		return cmd.Error("failed to create writer: %s", err2)
	}
	defer writer.Cleanup()

	if err3 := writer.AddFile(strings.NewReader(t1), "meta-data"); err3 != nil {
		panic(cmd.Error("failed to add file: %s", err3))
	}
	if err3 := writer.AddFile(strings.NewReader(t2), "user-data"); err3 != nil {
		panic(cmd.Error("failed to add file: %s", err3))
	}
	isoFile := iso.File()
	fd, err := isoFile.OpenFileForCreation()
	if err != nil {
		return cmd.Error("failed to open file: %s", err)
	}
	defer fd.Close()
	
	if err3 := writer.WriteTo(fd, "cidata"); err3 != nil {
		panic(cmd.Error("failed to write ISO image: %s", err))
	}
	vb := VboxFrom(iso.vm.Name())
	return vb.attacheDvdStorage(ctx, isoFile, "0")
}

/*
func (vbox *Vbox) attacheStorage(ctx context.Context, cachedFile newfs.File) error {
	_, err := vbox.execute(ctx, "storageattach", vbox.name,
		"--type", "dvddrive",
		"--storagectl", "IDE",
		"--port", "0",
		"--device", "1",
		"--medium", cachedFile.String())
	return err
}
*/

func (iso *Iso) File() newfs.File {
	isoFile := properties.VmFolder(iso.vm.Name()).ChildFile("seed.iso")
	return isoFile
}
func (iso *Iso) DetachAndDelete(ctx context.Context) *cmd.XbeeError {
	vb := VboxFrom(iso.vm.Name())
	if err := vb.detachDvdStorage(ctx, "0"); err != nil {
		return err
	}
	isoFile := iso.File()
	if err := isoFile.EnsureDelete(); err != nil {
		return cmd.Error("cannot ensure %s was deleted: %v", isoFile, err)
	}
	return nil
}

/*
func (vbox *Vbox) detachStorage(ctx context.Context) error {
	_, err := vbox.execute(ctx, "storageattach", vbox.name,
		"--type", "dvddrive",
		"--storagectl", "IDE",
		"--port", "0",
		"--device", "1",
		"--medium", "none")
	return err
}
*/
