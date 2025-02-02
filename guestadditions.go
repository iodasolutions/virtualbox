package virtualbox

import (
	"bytes"
	"context"
	"fmt"
	"github.com/iodasolutions/virtualbox/properties"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/template"
	"os/exec"
	"unicode"
)

var installAdditions = `#!/bin/bash
set -e
set -x 
function install_guest() {

apt update
apt install -y build-essential dkms linux-headers-$(uname -r)
mkdir -p /mnt/vbox
mount /dev/sr1 /mnt/vbox
set +e
#cette commande retourne un code error 2
/mnt/vbox/VBoxLinuxAdditions.run --nox11
set -e
apt-get purge -y build-essential
apt-get autoremove -y
umount /dev/sr1
rm -rf /mnt/vbox
echo {{ .version }} > /var/xbee/vbox_version


}

mkdir -p /var/xbee
if [ -f /var/xbee/vbox_version ]; then
	installed_version=$(cat /var/xbee/vbox_version)
	if [ "${installed_version}" != "{{ .version }}" ]; then
		install_guest
    fi
else
  install_guest
fi
`

func DownloadAndAttachGuestAdditions(ctx context.Context, vmName string) *cmd.XbeeError {
	vboxVersion := Version()
	url := fmt.Sprintf("https://download.virtualbox.org/virtualbox/%[1]s/VBoxGuestAdditions_%[1]s.iso", vboxVersion)
	if cachedFile, err := DownloadIfNotCached(ctx, url); err != nil {
		return err
	} else {
		vb := VboxFrom(vmName)
		return vb.attacheDvdStorage(ctx, cachedFile, "1")
	}
}

func DetachGuestAdditions(ctx context.Context, vmName string) *cmd.XbeeError {
	vb := VboxFrom(vmName)
	return vb.detachDvdStorage(ctx, "1")
}

func Version() string {
	aCmd := exec.Command(properties.VboxPath(), "--version")
	out, err := aCmd.Output()
	if err != nil {
		panic(cmd.Error("unexpected error while running %s --version : %v", properties.VboxPath(), err))
	}
	extendedVersion := string(out)
	return extractVersion(extendedVersion)
}

func extractVersion(s string) string {
	for i, r := range s {
		if !unicode.IsDigit(r) && r != '.' {
			return s[0:i]
		}
	}
	return ""
}

func GuestAdditionScript() string {
	model := map[string]interface{}{
		"version": Version(),
	}
	w := &bytes.Buffer{}
	if err := template.OutputWithTemplate(installAdditions, w, model, nil); err != nil {
		panic(cmd.Error("failed to parse userData template : %v", err))
	}
	return w.String()
}
