//go:build windows

package feed

import (
	"net/url"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// systemProxyURL reads the per-user proxy that Windows and most proxy tools
// configure. ProxyEnable is honored, so a disabled proxy is ignored.
func systemProxyURL(scheme string) *url.URL {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()
	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return nil
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return nil
	}
	return parseProxyServer(strings.TrimSpace(server), scheme)
}
