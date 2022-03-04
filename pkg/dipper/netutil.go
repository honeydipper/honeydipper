// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"net"
	"strings"
)

// GetIP : get first non loopback IP address.
func GetIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		panic(err)
	}
	for _, addr := range addrs {
		if v, ok := addr.(*net.IPNet); ok && !v.IP.IsLoopback() && v.IP.To4() != nil {
			ip := v.String()
			if slash := strings.IndexByte(ip, '/'); slash >= 0 {
				ip = ip[0:slash]
			}

			return ip
		}
	}

	return ""
}
