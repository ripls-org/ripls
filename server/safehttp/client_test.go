package safehttp

import (
	"net"
	"net/http"
	"testing"
)

func TestIsPubliclyRoutable(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want bool
	}{
		{"public v4", "8.8.8.8", true},
		{"public v4 alt", "151.101.1.69", true},
		{"public v6", "2606:4700:4700::1111", true},
		{"loopback v4", "127.0.0.1", false},
		{"loopback v4 high", "127.255.255.254", false},
		{"loopback v6", "::1", false},
		{"link-local v4 (gcp metadata)", "169.254.169.254", false},
		{"link-local v4 generic", "169.254.0.5", false},
		{"link-local v6", "fe80::1", false},
		{"rfc1918 10/8", "10.0.0.1", false},
		{"rfc1918 172.16/12 low", "172.16.0.1", false},
		{"rfc1918 172.16/12 high", "172.31.255.254", false},
		{"rfc1918 192.168/16", "192.168.1.1", false},
		{"cgnat low", "100.64.0.1", false},
		{"cgnat high", "100.127.255.254", false},
		{"cgnat boundary (public)", "100.128.0.1", true},
		{"public adjacent (100.63)", "100.63.255.254", true},
		{"unspecified v4", "0.0.0.0", false},
		{"unspecified v6", "::", false},
		{"broadcast", "255.255.255.255", false},
		{"multicast v4", "224.0.0.1", false},
		{"multicast v6", "ff02::1", false},
		{"ipv4-mapped loopback v6", "::ffff:127.0.0.1", false},
		{"ipv4-mapped private v6", "::ffff:10.0.0.1", false},
		{"ipv4-mapped public v6", "::ffff:8.8.8.8", true},
		{"ula v6", "fd00::1", false},
		{"nil", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			got := isPubliclyRoutable(ip)
			if got != tc.want {
				t.Errorf("isPubliclyRoutable(%q) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestSafeDialControl_RejectsNonTCP(t *testing.T) {
	err := safeDialControl("udp", "8.8.8.8:53", nil)
	if err == nil {
		t.Fatal("expected error for udp network")
	}
	if !IsSSRFBlockError(err) {
		t.Errorf("expected IsSSRFBlockError=true, got false (err=%v)", err)
	}
}

func TestSafeDialControl_RejectsPrivateAddr(t *testing.T) {
	err := safeDialControl("tcp", "10.0.0.5:443", nil)
	if err == nil {
		t.Fatal("expected error for rfc1918 address")
	}
	if !IsSSRFBlockError(err) {
		t.Errorf("expected IsSSRFBlockError=true, got false (err=%v)", err)
	}
}

func TestSafeDialControl_AllowsPublicAddr(t *testing.T) {
	if err := safeDialControl("tcp", "8.8.8.8:443", nil); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestNewClient_CheckRedirect_DefaultHTTPSOnly(t *testing.T) {
	c := NewClient()
	if c.CheckRedirect == nil {
		t.Fatal("expected CheckRedirect to be set")
	}
	// http redirect should be rejected by default (https-only).
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	if err := c.CheckRedirect(req, []*http.Request{{}}); err == nil {
		t.Error("expected http redirect target to be rejected under default https-only policy")
	}
	// https redirect should be allowed.
	httpsReq, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err := c.CheckRedirect(httpsReq, []*http.Request{{}}); err != nil {
		t.Errorf("expected https redirect to be allowed, got %v", err)
	}
}

func TestNewClient_AllowsHTTPRedirectWhenConfigured(t *testing.T) {
	c := NewClient(WithRedirectSchemes("http", "https"))
	// http redirect should now be allowed.
	httpReq, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	if err := c.CheckRedirect(httpReq, []*http.Request{{}}); err != nil {
		t.Errorf("expected http redirect to be allowed with http+https scheme config, got %v", err)
	}
	// https redirect should also be allowed.
	httpsReq, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err := c.CheckRedirect(httpsReq, []*http.Request{{}}); err != nil {
		t.Errorf("expected https redirect to be allowed, got %v", err)
	}
}

func TestNewClient_CheckRedirect_MaxRedirects(t *testing.T) {
	c := NewClient(WithMaxRedirects(3))
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	prev := make([]*http.Request, 3)
	if err := c.CheckRedirect(req, prev); err == nil {
		t.Error("expected error when redirect count reaches limit")
	}
}

func TestIsHostPubliclyRoutable(t *testing.T) {
	cases := []struct {
		name string
		host string
		want bool
	}{
		{"hostname (not literal IP)", "example.com", true},
		{"localhost hostname", "localhost", true}, // hostname, not literal IP
		{"loopback literal", "127.0.0.1", false},
		{"private literal", "10.0.0.1", false},
		{"link-local literal", "169.254.169.254", false},
		{"cgnat literal", "100.64.1.1", false},
		{"public literal", "8.8.8.8", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsHostPubliclyRoutable(tc.host)
			if got != tc.want {
				t.Errorf("IsHostPubliclyRoutable(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}
