package simulation

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
)

// Client wraps authenticated RPC clients with simulation timestamp injection.
// Use SetTimestamp to control the simulated time, and AsUser to switch between
// authenticated user contexts.
type Client struct {
	serverURL string
	transport *simulationTransport

	// Cached service clients (lazily created).
	mu        sync.Mutex
	loginSvc  apiconnect.LoginServiceClient
	gearSvc   apiconnect.GearServiceClient
	commSvc   apiconnect.CommunityServiceClient
	transSvc  apiconnect.TransferServiceClient
	reqSvc    apiconnect.RequestServiceClient
	expSvc    apiconnect.ExperienceServiceClient
	chatSvc   apiconnect.ChatServiceClient
	mediaSvc  apiconnect.MediaServiceClient
	adminSvc  apiconnect.AdminServiceClient
	userSvc   apiconnect.UserServiceClient
	searchSvc apiconnect.SearchServiceClient
	impactSvc apiconnect.ImpactServiceClient
	locSvc    apiconnect.LocationServiceClient
	feedSvc   apiconnect.FeedServiceClient
}

// NewClient creates a new simulation client for the given server URL.
func NewClient(serverURL string) *Client {
	return &Client{
		serverURL: serverURL,
		transport: &simulationTransport{
			base: http.DefaultTransport,
		},
	}
}

// SetTimestamp sets the simulation timestamp for all subsequent requests.
func (c *Client) SetTimestamp(t time.Time) {
	c.transport.setTimestamp(t)
}

// AsUser switches the client to authenticate as the given user.
func (c *Client) AsUser(token string) {
	c.transport.setToken(token)
}

// CurrentToken returns the current auth token.
func (c *Client) CurrentToken() string {
	c.transport.mu.RLock()
	defer c.transport.mu.RUnlock()
	return c.transport.token
}

// httpClient returns an http.Client using the simulation transport.
func (c *Client) httpClient() *http.Client {
	return &http.Client{Transport: c.transport}
}

// Login returns the LoginService client (no auth needed for registration).
func (c *Client) Login() apiconnect.LoginServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loginSvc == nil {
		c.loginSvc = apiconnect.NewLoginServiceClient(c.httpClient(), c.serverURL)
	}
	return c.loginSvc
}

// Gear returns the GearService client.
func (c *Client) Gear() apiconnect.GearServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gearSvc == nil {
		c.gearSvc = apiconnect.NewGearServiceClient(c.httpClient(), c.serverURL)
	}
	return c.gearSvc
}

// Community returns the CommunityService client.
func (c *Client) Community() apiconnect.CommunityServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.commSvc == nil {
		c.commSvc = apiconnect.NewCommunityServiceClient(c.httpClient(), c.serverURL)
	}
	return c.commSvc
}

// Transfer returns the TransferService client.
func (c *Client) Transfer() apiconnect.TransferServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.transSvc == nil {
		c.transSvc = apiconnect.NewTransferServiceClient(c.httpClient(), c.serverURL)
	}
	return c.transSvc
}

// Request returns the RequestService client.
func (c *Client) Request() apiconnect.RequestServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reqSvc == nil {
		c.reqSvc = apiconnect.NewRequestServiceClient(c.httpClient(), c.serverURL)
	}
	return c.reqSvc
}

// Experience returns the ExperienceService client.
func (c *Client) Experience() apiconnect.ExperienceServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.expSvc == nil {
		c.expSvc = apiconnect.NewExperienceServiceClient(c.httpClient(), c.serverURL)
	}
	return c.expSvc
}

// Chat returns the ChatService client.
func (c *Client) Chat() apiconnect.ChatServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.chatSvc == nil {
		c.chatSvc = apiconnect.NewChatServiceClient(c.httpClient(), c.serverURL)
	}
	return c.chatSvc
}

// Media returns the MediaService client.
func (c *Client) Media() apiconnect.MediaServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mediaSvc == nil {
		c.mediaSvc = apiconnect.NewMediaServiceClient(c.httpClient(), c.serverURL)
	}
	return c.mediaSvc
}

// Admin returns the AdminService client.
func (c *Client) Admin() apiconnect.AdminServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.adminSvc == nil {
		c.adminSvc = apiconnect.NewAdminServiceClient(c.httpClient(), c.serverURL)
	}
	return c.adminSvc
}

// User returns the UserService client.
func (c *Client) User() apiconnect.UserServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.userSvc == nil {
		c.userSvc = apiconnect.NewUserServiceClient(c.httpClient(), c.serverURL)
	}
	return c.userSvc
}

// Search returns the SearchService client.
func (c *Client) Search() apiconnect.SearchServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.searchSvc == nil {
		c.searchSvc = apiconnect.NewSearchServiceClient(c.httpClient(), c.serverURL)
	}
	return c.searchSvc
}

// Impact returns the ImpactService client.
func (c *Client) Impact() apiconnect.ImpactServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.impactSvc == nil {
		c.impactSvc = apiconnect.NewImpactServiceClient(c.httpClient(), c.serverURL)
	}
	return c.impactSvc
}

// Location returns the LocationService client.
func (c *Client) Location() apiconnect.LocationServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.locSvc == nil {
		c.locSvc = apiconnect.NewLocationServiceClient(c.httpClient(), c.serverURL)
	}
	return c.locSvc
}

// Feed returns the FeedService client.
func (c *Client) Feed() apiconnect.FeedServiceClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.feedSvc == nil {
		c.feedSvc = apiconnect.NewFeedServiceClient(c.httpClient(), c.serverURL)
	}
	return c.feedSvc
}

// simulationTransport injects authentication and simulation timestamp headers.
type simulationTransport struct {
	base http.RoundTripper

	mu        sync.RWMutex
	token     string
	timestamp *time.Time
}

// setToken sets the auth token for subsequent requests.
func (t *simulationTransport) setToken(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.token = token
}

// setTimestamp sets the simulation timestamp for subsequent requests.
func (t *simulationTransport) setTimestamp(ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timestamp = &ts
}

// RoundTrip implements http.RoundTripper, injecting auth and simulation headers.
func (t *simulationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.RLock()
	token := t.token
	ts := t.timestamp
	t.mu.RUnlock()

	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}
	if ts != nil {
		req.Header.Set(clock.SimulationTimestampHeader, strconv.FormatInt(ts.Unix(), 10))
	}
	return t.base.RoundTrip(req)
}
