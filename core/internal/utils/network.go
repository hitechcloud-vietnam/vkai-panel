package utils

import (
	"net"
	"net/http"
	"strings"
)

// GetClientIP gets the client IP address from request
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	xRealIP := r.Header.Get("X-Real-IP")
	if xRealIP != "" {
		return xRealIP
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// IsPrivateIP checks if an IP is private
func IsPrivateIP(ip string) bool {
	privateIPBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, block := range privateIPBlocks {
		_, cidr, err := net.ParseCIDR(block)
		if err != nil {
			continue
		}
		if cidr.Contains(parsedIP) {
			return true
		}
	}
	return false
}

// IsPublicIP checks if an IP is public
func IsPublicIP(ip string) bool {
	return !IsPrivateIP(ip)
}

// IsValidIP checks if an IP is valid
func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsValidIPv4 checks if an IPv4 is valid
func IsValidIPv4(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil && parsedIP.To4() != nil
}

// IsValidIPv6 checks if an IPv6 is valid
func IsValidIPv6(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil && parsedIP.To4() == nil
}

// GetLocalIP gets the local IP address
func GetLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// GetLocalIPs gets all local IP addresses
func GetLocalIPs() ([]string, error) {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			ips = append(ips, ipNet.IP.String())
		}
	}
	return ips, nil
}

// IsPortAvailable checks if a port is available
func IsPortAvailable(port int) bool {
	addr := net.JoinHostPort("", string(rune(port+'0')))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// GetAvailablePort gets an available port
func GetAvailablePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port, nil
}

// IsDomainReachable checks if a domain is reachable
func IsDomainReachable(domain string) bool {
	_, err := net.LookupHost(domain)
	return err == nil
}

// GetMXRecords gets MX records for a domain
func GetMXRecords(domain string) ([]*net.MX, error) {
	return net.LookupMX(domain)
}

// GetCNAME gets CNAME record for a domain
func GetCNAME(domain string) (string, error) {
	return net.LookupCNAME(domain)
}

// GetARecords gets A records for a domain
func GetARecords(domain string) ([]string, error) {
	return net.LookupHost(domain)
}

// IsIPAddress checks if a string is an IP address
func IsIPAddress(s string) bool {
	return net.ParseIP(s) != nil
}

// IsCIDR checks if a string is a CIDR notation
func IsCIDR(s string) bool {
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

// ParseCIDR parses a CIDR notation
func ParseCIDR(s string) (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(s)
}

// IPInRange checks if an IP is in a CIDR range
func IPInRange(ip string, cidr string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	return ipNet.Contains(parsedIP)
}
