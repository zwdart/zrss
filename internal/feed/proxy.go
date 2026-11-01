package feed

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// proxyFromSettings picks a proxy for req: the environment first, then the
// operating system's settings. Loopback requests never use a proxy, so local
// test servers keep working when a proxy is configured.
func proxyFromSettings(req *http.Request) (*url.URL, error) {
	if u, err := http.ProxyFromEnvironment(req); err != nil || u != nil {
		return u, err
	}
	if req == nil || req.URL == nil || isLoopbackHost(req.URL.Hostname()) {
		return nil, nil
	}
	return systemProxyURL(req.URL.Scheme), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// parseProxyServer understands the forms Windows stores in ProxyServer:
// "host:port", or per-scheme pairs such as
// "http=host:port;https=host:port;socks=host:port".
func parseProxyServer(server, scheme string) *url.URL {
	server = strings.TrimSpace(server)
	if server == "" {
		return nil
	}
	if !strings.Contains(server, "=") {
		return proxyURL(server)
	}
	var fallback, socks string
	for _, part := range strings.Split(server, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case strings.ToLower(scheme):
			return proxyURL(value)
		case "http":
			if fallback == "" {
				fallback = value
			}
		case "socks", "socks5":
			socks = value
		}
	}
	if fallback != "" {
		return proxyURL(fallback)
	}
	if socks != "" {
		return proxyURL("socks5://" + socks)
	}
	return nil
}

// proxyURL turns a proxy address into a URL, defaulting to HTTP because that
// is what Windows and local proxy tools usually configure.
func proxyURL(address string) *url.URL {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil
	}
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}
