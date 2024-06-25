package main

import "net"

// isTrustedIp checks whether an IP is inside a network (CIDR) or not
func isTrustedIp(trustedNetworks []*net.IPNet, ip net.IP) (result bool) {
	for _, trustedCidrPtr := range trustedNetworks {
		if trustedCidrPtr.Contains(ip) {
			result = true
			break
		}
	}

	return result
}
