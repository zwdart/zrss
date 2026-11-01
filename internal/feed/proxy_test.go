package feed

import "testing"

func TestParseProxyServer(t *testing.T) {
	cases := []struct {
		name   string
		server string
		scheme string
		want   string
	}{
		{"plain address", "127.0.0.1:7890", "https", "http://127.0.0.1:7890"},
		{"with scheme", "http://proxy.example:8080", "http", "http://proxy.example:8080"},
		{"per scheme", "http=127.0.0.1:7890;https=127.0.0.1:7891", "https", "http://127.0.0.1:7891"},
		{"falls back to HTTP", "http=127.0.0.1:7890", "https", "http://127.0.0.1:7890"},
		{"socks", "socks=127.0.0.1:1080", "https", "socks5://127.0.0.1:1080"},
		{"empty", "  ", "https", ""},
		{"broken", "http://[", "https", ""},
	}
	for _, tc := range cases {
		got := parseProxyServer(tc.server, tc.scheme)
		gotText := ""
		if got != nil {
			gotText = got.String()
		}
		if gotText != tc.want {
			t.Errorf("%s: parseProxyServer(%q, %q) = %q, want %q", tc.name, tc.server, tc.scheme, gotText, tc.want)
		}
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for _, host := range []string{"localhost", "LOCALHOST", "127.0.0.1", "::1"} {
		if !isLoopbackHost(host) {
			t.Errorf("isLoopbackHost(%q) = false, want true", host)
		}
	}
	for _, host := range []string{"example.com", "192.168.1.2", ""} {
		if isLoopbackHost(host) {
			t.Errorf("isLoopbackHost(%q) = true, want false", host)
		}
	}
}
