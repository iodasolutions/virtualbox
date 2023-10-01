package virtualbox

import (
	"fmt"
	"net"
	"strconv"
	"sync"
)

var NextPort = &nextPort{
	portsGiven: make(map[int]int),
}

type nextPort struct {
	portsGiven map[int]int
	sync.Mutex
}

func (np *nextPort) isPortAvailable(port int) bool {
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return true
	}
	defer conn.Close()
	return false
}

func (np *nextPort) NextFreePort(firstPort string) string {
	if firstPort == "" {
		firstPort = "1025"
	}
	if aPort, err := strconv.Atoi(firstPort); err == nil {
		for i := aPort; i < 65536; i++ {
			if np.isPortAvailable(i) {
				if _, ok := np.portsGiven[i]; !ok {
					np.portsGiven[i] = i
					return strconv.Itoa(i)
				}
			}
		}
	} else {
		panic(fmt.Errorf("Param FirstPort %s cannot be converted to int", firstPort))
	}
	return ""
}
