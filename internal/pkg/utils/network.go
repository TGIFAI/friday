package utils

import "net"

// IsPrivateHost checks whether a hostname resolves to a private/loopback address.
// Used for SSRF protection across all HTTP-related tools.
func IsPrivateHost(host string) bool {
	// Check well-known private hostnames first.
	if host == "localhost" || host == "metadata.google.internal" {
		return true
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS fails, check if it's a raw IP.
		ip := net.ParseIP(host)
		if ip == nil {
			return false
		}
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}
