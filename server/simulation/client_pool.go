package simulation

import "time"

// ClientPool holds a fixed-token Client per user email.
//
// Each email maps to its own Client with its own simulationTransport and a
// token that is set once at pool construction and never changes. Callers
// never call AsUser on a pool client: the answer to "which user am I?" is
// the choice of which Client you dispatch on, not the state of a shared
// object. This removes a class of bug where cross-user side operations
// (owner-side re-share, chat injection) would silently leave the shared
// client authenticated as the wrong user.
//
// The sequential and concurrent executors both build their pool via
// NewClientPool and look up callers via For(email).
type ClientPool struct {
	serverURL string
	clients   map[string]*Client
}

// NewClientPool builds a pool of per-user clients from the given email→token
// map. Each client gets its own transport; tokens are fixed.
func NewClientPool(serverURL string, userTokens map[string]string) *ClientPool {
	p := &ClientPool{
		serverURL: serverURL,
		clients:   make(map[string]*Client, len(userTokens)),
	}
	for email, token := range userTokens {
		if token == "" {
			continue
		}
		c := NewClient(serverURL)
		c.AsUser(token)
		p.clients[email] = c
	}
	return p
}

// For returns the client authenticated as the given email, or nil if the
// user is not in the pool.
func (p *ClientPool) For(email string) *Client {
	if p == nil {
		return nil
	}
	return p.clients[email]
}

// SetTimestamp applies the given simulation timestamp to every client in the
// pool. Use at the top of each sequential step so all RPCs that run for that
// step carry the same simulated time.
func (p *ClientPool) SetTimestamp(t time.Time) {
	if p == nil {
		return
	}
	for _, c := range p.clients {
		c.SetTimestamp(t)
	}
}
