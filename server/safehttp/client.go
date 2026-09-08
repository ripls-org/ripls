package safehttp

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"syscall"
	"time"
)

// errUnsafeOutboundAddr is returned by the safe-outbound dialer when the
// resolved address points at a private, loopback, link-local, or
// otherwise SSRF-sensitive range. Carried inside the wrapped dial error
// returned to http.Client.Do.
var errUnsafeOutboundAddr = errors.New("outbound address blocked")

// config holds the options for NewClient.
type config struct {
	timeout         time.Duration
	redirectSchemes []string
	jar             *cookiejar.Jar
	maxRedirects    int
}

// Option configures a NewClient call.
type Option func(*config)

// WithTimeout sets the per-request timeout and transport-level timeouts.
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		c.timeout = d
	}
}

// WithRedirectSchemes sets the URL schemes allowed in redirect responses.
// Defaults to ["https"] when not set, so callers that fetch user-supplied
// URLs over both http and https must pass WithRedirectSchemes("http", "https").
func WithRedirectSchemes(schemes ...string) Option {
	return func(c *config) {
		c.redirectSchemes = schemes
	}
}

// WithCookieJar attaches a cookie jar to the built client.
func WithCookieJar(jar *cookiejar.Jar) Option {
	return func(c *config) {
		c.jar = jar
	}
}

// WithMaxRedirects sets the maximum number of redirects to follow. Default 5.
func WithMaxRedirects(n int) Option {
	return func(c *config) {
		c.maxRedirects = n
	}
}

// NewClient builds an *http.Client suitable for fetching user-supplied URLs.
// Guards against SSRF in three layers:
//
//  1. A net.Dialer.Control callback runs after DNS resolution and rejects
//     loopback, broadcast, multicast, unspecified, link-local (covers cloud
//     metadata endpoints at 169.254.169.254), CGNAT, and RFC1918 addresses,
//     plus their IPv6 equivalents (incl. ULA fc00::/7 and IPv4-mapped v6).
//     DNS rebinding is caught here — the IP at connect time is evaluated.
//  2. A CheckRedirect callback re-asserts that every hop uses an allowed
//     scheme, preventing http:// or file:// redirect escapes.
//  3. A tight per-connection timeout caps the lifetime of any one fetch.
func NewClient(opts ...Option) *http.Client {
	cfg := &config{
		timeout:         30 * time.Second,
		redirectSchemes: []string{"https"},
		maxRedirects:    5,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	dialer := &net.Dialer{
		Timeout:   cfg.timeout,
		KeepAlive: -1,
		Control:   safeDialControl,
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   cfg.timeout,
		ResponseHeaderTimeout: cfg.timeout,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          1,
		MaxConnsPerHost:       2,
		IdleConnTimeout:       30 * time.Second,
		DisableKeepAlives:     true,
	}

	allowedSchemes := make(map[string]bool, len(cfg.redirectSchemes))
	for _, s := range cfg.redirectSchemes {
		allowedSchemes[s] = true
	}
	maxRedirects := cfg.maxRedirects

	client := &http.Client{
		Timeout:   cfg.timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects (%d)", maxRedirects)
			}
			if !allowedSchemes[req.URL.Scheme] {
				return fmt.Errorf("redirect scheme %q not allowed", req.URL.Scheme)
			}
			return nil
		},
	}
	if cfg.jar != nil {
		client.Jar = cfg.jar
	}
	return client
}

// IsSSRFBlockError reports whether err was caused by the SSRF guard
// rejecting the destination address. Callers use this to map the failure
// to CodeInvalidArgument rather than the generic CodeUnavailable.
func IsSSRFBlockError(err error) bool {
	return errors.Is(err, errUnsafeOutboundAddr)
}

// IsHostPubliclyRoutable reports whether host is safe to dial. If host is
// not a literal IP address, it returns true — the DNS-resolved address is
// validated by the dialer Control hook at connect time. If host is a
// literal IP, it returns false for loopback, link-local, private, and
// other non-public ranges.
func IsHostPubliclyRoutable(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return true // not a literal IP; DNS + dialer will verify
	}
	return isPubliclyRoutable(ip)
}

// safeDialControl is the net.Dialer.Control hook used by NewClient. It
// runs once per connection attempt, after DNS resolution, with the
// destination address already in IP form.
func safeDialControl(network, address string, _ syscall.RawConn) error {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return fmt.Errorf("%w: unsupported network %q", errUnsafeOutboundAddr, network)
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: bad address %q: %w", errUnsafeOutboundAddr, address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Dialer hands us a resolved IP; a nil parse here would be a
		// programming error in net/http. Treat as unsafe.
		return fmt.Errorf("%w: cannot parse %q as IP", errUnsafeOutboundAddr, host)
	}
	if !isPubliclyRoutable(ip) {
		return fmt.Errorf("%w: %s is not publicly routable", errUnsafeOutboundAddr, ip)
	}
	return nil
}

// isPubliclyRoutable reports whether ip is safe to dial from a server
// that must not reach internal infrastructure. Rejects loopback, link-
// local (incl. cloud metadata at 169.254.169.254), broadcast, multicast,
// unspecified, private (RFC1918), CGNAT (RFC6598 100.64/10), and IPv6
// ULAs (fc00::/7). IPv4-mapped IPv6 addresses (::ffff:a.b.c.d) are
// unwrapped and re-checked as v4.
func isPubliclyRoutable(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() ||
		ip.IsUnspecified() || ip.IsPrivate() {
		return false
	}
	// IsPrivate covers RFC1918 + RFC4193 ULAs. CGNAT is not in IsPrivate;
	// check the 100.64.0.0/10 range explicitly.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1]&0xc0 == 64 {
			return false
		}
		// 0.0.0.0/8 source-only and 255.255.255.255 broadcast.
		if v4[0] == 0 || ip.Equal(net.IPv4bcast) {
			return false
		}
	}
	return true
}
