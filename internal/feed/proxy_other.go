//go:build !windows

package feed

import "net/url"

// systemProxyURL is a Windows-only setting; other systems use the proxy
// environment variables that http.ProxyFromEnvironment reads.
func systemProxyURL(string) *url.URL { return nil }
