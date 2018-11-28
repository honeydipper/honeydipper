package dipper

import (
	"net"
)

// GetIP : get first non loopback IP address
func GetIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			panic(err)
		}
		for _, addr := range addrs {
			if v, ok := addr.(*net.IPAddr); ok && !v.IP.IsLoopback() {
				return v.String()
			}
		}
	}
	return ""
}
