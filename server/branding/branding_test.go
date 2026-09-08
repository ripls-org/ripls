package branding

import "testing"

func TestBaseURLFromHostname(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     string
	}{
		{"empty stays empty", "", ""},
		{"bare host gets https", "app.example.com", "https://app.example.com"},
		{"whitespace is trimmed", "  app.example.com  ", "https://app.example.com"},
		{"trailing slash is dropped", "app.example.com/", "https://app.example.com"},
		{"explicit https is kept", "https://app.example.com", "https://app.example.com"},
		{"explicit http is kept", "http://app.example.com", "http://app.example.com"},

		// Dev and e2e targets have no TLS in front of them, so forcing https
		// would make every mailed link unreachable in those environments.
		{"localhost gets http", "localhost:8080", "http://localhost:8080"},
		{"loopback IP gets http", "127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"android emulator host gets http", "10.0.2.2:8080", "http://10.0.2.2:8080"},

		// A public host that merely *starts* with a local-looking label must
		// still get https.
		{"host with local-looking prefix still gets https", "localhost.example.com", "https://localhost.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BaseURLFromHostname(tt.hostname); got != tt.want {
				t.Errorf("BaseURLFromHostname(%q) = %q, want %q", tt.hostname, got, tt.want)
			}
		})
	}
}

func TestAppURL(t *testing.T) {
	b := Config{AppBaseURL: "https://app.example.com"}

	if got, want := b.AppURL("/reset-password?token=abc"), "https://app.example.com/reset-password?token=abc"; got != want {
		t.Errorf("AppURL = %q, want %q", got, want)
	}
	if got, want := b.AppURL("reset-password"), "https://app.example.com/reset-password"; got != want {
		t.Errorf("AppURL without leading slash = %q, want %q", got, want)
	}

	// A base URL with a trailing slash must not produce a doubled separator.
	withSlash := Config{AppBaseURL: "https://app.example.com/"}
	if got, want := withSlash.AppURL("/go/abc"), "https://app.example.com/go/abc"; got != want {
		t.Errorf("AppURL with trailing-slash base = %q, want %q", got, want)
	}

	// Unconfigured returns empty rather than a relative path: a relative URL in
	// an email is unclickable, so the caller needs to be able to omit the link.
	if got := (Config{}).AppURL("/go/abc"); got != "" {
		t.Errorf("AppURL with no base = %q, want empty", got)
	}
}

func TestDeepLink(t *testing.T) {
	b := Config{DeepLinkScheme: "ripls"}
	if got, want := b.DeepLink("invite?token=abc"), "ripls://invite?token=abc"; got != want {
		t.Errorf("DeepLink = %q, want %q", got, want)
	}
	if got, want := b.DeepLink("/invite"), "ripls://invite"; got != want {
		t.Errorf("DeepLink with leading slash = %q, want %q", got, want)
	}
	if got := (Config{}).DeepLink("invite"); got != "" {
		t.Errorf("DeepLink with no scheme = %q, want empty", got)
	}
}

func TestBotUserAgent(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			"derived from name and base URL",
			Config{Name: "Ripls", AppBaseURL: "https://app.example.com"},
			"RiplsBot/1.0 (+https://app.example.com)",
		},
		{
			"explicit override wins",
			Config{Name: "Ripls", AppBaseURL: "https://app.example.com", UserAgentOverride: "CustomBot/2.0"},
			"CustomBot/2.0",
		},
		{
			"no base URL omits the contact suffix",
			Config{Name: "Ripls"},
			"RiplsBot/1.0",
		},
		{
			// A User-Agent with a space in the product token is malformed, so a
			// multi-word brand collapses rather than emitting one.
			"spaces are removed from the name",
			Config{Name: "Two Words", AppBaseURL: "https://app.example.com"},
			"TwoWordsBot/1.0 (+https://app.example.com)",
		},
		{
			// Zero value: services get a usable agent without wiring branding,
			// which is what keeps the Set* injection optional in tests.
			"zero value still identifies itself",
			Config{},
			"AppBot/1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.BotUserAgent(); got != tt.want {
				t.Errorf("BotUserAgent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"zero value is valid", Config{}, false},
		{"absolute URLs are valid", Config{
			PrivacyPolicyURL: "https://example.com/privacy",
			TermsURL:         "https://example.com/terms",
			AppStoreURL:      "https://apps.apple.com/app/id1",
			PlayStoreURL:     "https://play.google.com/store/apps/details?id=com.example",
		}, false},

		// The failure this catches: a bare hostname renders as a relative link
		// and silently 404s on whatever page it appears on.
		{"bare hostname is rejected", Config{PrivacyPolicyURL: "example.com/privacy"}, true},
		{"scheme-relative URL is rejected", Config{TermsURL: "//example.com/terms"}, true},
		{"path-only URL is rejected", Config{AppStoreURL: "/download"}, true},

		{"valid scheme is accepted", Config{DeepLinkScheme: "myapp"}, false},
		{"scheme with :// is rejected", Config{DeepLinkScheme: "myapp://"}, true},
		{"scheme with a colon is rejected", Config{DeepLinkScheme: "myapp:"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Error("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}
