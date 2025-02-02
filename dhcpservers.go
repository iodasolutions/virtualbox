package virtualbox

import (
	"context"
	"fmt"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/provider"
	"strconv"
	"strings"
)

type DhcpServer struct {
	NetworkName    string
	IP             string
	NetworkMask    string
	LowerIPAddress string
	UpperIPAddress string
	Enabled        bool
}

type DhcpServers struct {
	byName  map[string]*DhcpServer
	byIndex map[int]*DhcpServer
}

func DefaultNet() string {
	return fmt.Sprintf("default_%s", provider.EnvId())
}

func (ds *DhcpServers) HasName(name string) bool {
	_, ok := ds.byName[name]
	return ok
}
func (ds *DhcpServers) nextAvailableIndex() string {
	for i := 99; i < 255; i++ {
		if _, ok := ds.byIndex[i]; !ok {
			return strconv.Itoa(i)
		}
	}
	return ""
}

func (ds *DhcpServers) CreateNewDhcpIfNecessary(ctx context.Context) *cmd.XbeeError {
	xbeenetName := DefaultNet()
	if !ds.HasName(xbeenetName) {
		ind := ds.nextAvailableIndex()
		//VBoxManage natnetwork add --netname "InternalNet" --network "192.168.100.0/24" --enable
		if _, err := VboxFrom("").execute(ctx, "natnetwork",
			"add", "--netname", xbeenetName, "--network", fmt.Sprintf("192.168.%s.0/24", ind), "--enable"); err != nil {
			return err
		}
		d := &DhcpServer{
			NetworkName:    xbeenetName,
			IP:             fmt.Sprintf("192.168.%s.1", ind),
			NetworkMask:    "255.255.255.0",
			LowerIPAddress: fmt.Sprintf("192.168.%s.2", ind),
			UpperIPAddress: fmt.Sprintf("192.168.%s.254", ind),
			Enabled:        true,
		}
		_, err := VboxFrom("").execute(ctx, "dhcpserver", "add",
			"--netname", d.NetworkName,
			"--ip", d.IP,
			"--netmask", d.NetworkMask,
			"--lowerip", d.LowerIPAddress,
			"--upperip", d.UpperIPAddress,
			"--enable")
		if err != nil {
			return err
		}
	}
	return nil
}

func EnsureXbeenetExist(ctx context.Context) *cmd.XbeeError {
	vboxLock.Lock()
	defer vboxLock.Unlock()
	dhcps, err := NewDhcpServers(ctx)
	if err != nil {
		return err
	}
	return dhcps.CreateNewDhcpIfNecessary(ctx)
}
func EnsureXbeenetDeleted(ctx context.Context) *cmd.XbeeError {
	dhcps, err := NewDhcpServers(ctx)
	if err != nil {
		return err
	}
	xbeenetName := DefaultNet()
	if dhcps.HasName(xbeenetName) {
		if err := VboxFrom("").removeDhcpServer(ctx, xbeenetName); err != nil {
			return err
		}
		if err := VboxFrom("").remoteIntNet(ctx); err != nil {
			return err
		}
	}
	return nil
}

func NewDhcpServers(ctx context.Context) (result *DhcpServers, err *cmd.XbeeError) {
	result = &DhcpServers{
		byName:  map[string]*DhcpServer{},
		byIndex: map[int]*DhcpServer{},
	}
	var out string
	out, err = VboxFrom("").listDhcpServers(ctx)
	if err != nil {
		return
	}
	parser := &Parser{content: out}
	for _, aMap := range parser.asList() {
		dhcp := &DhcpServer{
			NetworkName:    aMap["NetworkName"],
			IP:             aMap["Dhcpd IP"],
			NetworkMask:    aMap["NetworkMask"],
			LowerIPAddress: aMap["LowerIPAddress"],
			UpperIPAddress: aMap["UpperIPAddress"],
		}
		if aMap["Enabled"] == "Yes" {
			dhcp.Enabled = true
		}
		index, _ := strconv.Atoi(strings.Split(dhcp.IP, ".")[2])
		result.byName[dhcp.NetworkName] = dhcp
		result.byIndex[index] = dhcp
	}
	return
}

func (d *DhcpServer) String() string {
	list := []string{
		fmt.Sprintf("NetworkName:%s", d.NetworkName),
		fmt.Sprintf("IP:%s", d.IP),
		fmt.Sprintf("NetworkMask:%s", d.NetworkMask),
		fmt.Sprintf("LowerIPAddress:%s", d.LowerIPAddress),
		fmt.Sprintf("UpperIPAddress:%s", d.UpperIPAddress),
		fmt.Sprintf("Enabled:%t", d.Enabled),
	}
	return strings.Join(list, "\n")
}
