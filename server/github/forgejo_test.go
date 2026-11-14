package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestForgejoConfigIsComplete(t *testing.T) {
	full := ForgejoConfig{BaseURL: "https://code.example.org", Token: "t", Owner: "o", Repo: "r"}
	if !full.IsComplete() {
		t.Fatal("a full config must be complete")
	}
	for name, strip := range map[string]func(*ForgejoConfig){
		"base URL": func(c *ForgejoConfig) { c.BaseURL = "" },
		"token":    func(c *ForgejoConfig) { c.Token = "" },
		"owner":    func(c *ForgejoConfig) { c.Owner = "" },
		"repo":     func(c *ForgejoConfig) { c.Repo = "" },
	} {
		cfg := full
		strip(&cfg)
		if cfg.IsComplete() {
			t.Errorf("config without %s must not be complete", name)
		}
		if _, err := NewForgejoClient(cfg); err == nil {
			t.Errorf("NewForgejoClient must reject a config without %s", name)
		}
	}
}

func TestNewForgejoClientRejectsBadBaseURL(t *testing.T) {
	for _, base := range []string{"code.example.org", "ftp://code.example.org", "https://"} {
		_, err := NewForgejoClient(ForgejoConfig{BaseURL: base, Token: "t", Owner: "o", Repo: "r"})
		if err == nil {
			t.Errorf("base URL %q must be rejected", base)
		}
	}
}

// forgejoFake is a Forgejo API holding one repository's labels and recording
// what the client sent.
type forgejoFake struct {
	t             *testing.T
	labels        []forgejoLabel
	nextLabelID   int64
	createdLabels []string
	issue         map[string]any
	auth          []string
}

func (f *forgejoFake) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/repos/acme/tracker/labels", func(w http.ResponseWriter, r *http.Request) {
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		page := r.URL.Query().Get("page")
		start := 0
		if page == "2" {
			start = forgejoLabelPageSize
		}
		end := min(start+forgejoLabelPageSize, len(f.labels))
		batch := []forgejoLabel{}
		if start < len(f.labels) {
			batch = f.labels[start:end]
		}
		_ = json.NewEncoder(w).Encode(batch)
	})
	mux.HandleFunc("POST /api/v1/repos/acme/tracker/labels", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			f.t.Errorf("label body: %v", err)
		}
		if in.Color != forgejoNewLabelColor {
			f.t.Errorf("new label color = %q, want %q", in.Color, forgejoNewLabelColor)
		}
		f.nextLabelID++
		f.createdLabels = append(f.createdLabels, in.Name)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(forgejoLabel{ID: f.nextLabelID, Name: in.Name})
	})
	mux.HandleFunc("POST /api/v1/repos/acme/tracker/issues", func(w http.ResponseWriter, r *http.Request) {
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		if err := json.NewDecoder(r.Body).Decode(&f.issue); err != nil {
			f.t.Errorf("issue body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"number": 7, "html_url": "https://code.example.org/acme/tracker/issues/7"})
	})
	return mux
}

func newForgejoTestClient(t *testing.T, fake *forgejoFake) *ForgejoClient {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	// A trailing slash on the base URL must not double up in request paths.
	client, err := NewForgejoClient(ForgejoConfig{BaseURL: srv.URL + "/", Token: "secret-token", Owner: "acme", Repo: "tracker"})
	if err != nil {
		t.Fatalf("NewForgejoClient: %v", err)
	}
	return client
}

func TestForgejoCreateIssueResolvesLabelsAndAppliesTypeAsLabel(t *testing.T) {
	fake := &forgejoFake{t: t, labels: []forgejoLabel{{ID: 3, Name: "user-feedback"}}, nextLabelID: 100}
	client := newForgejoTestClient(t, fake)

	issue, err := client.CreateIssue(context.Background(), "", "", "It broke", "details", "Bug", []string{"user-feedback"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if issue.Number != 7 || issue.URL != "https://code.example.org/acme/tracker/issues/7" {
		t.Errorf("issue = %+v", issue)
	}
	if fake.issue["title"] != "It broke" || fake.issue["body"] != "details" {
		t.Errorf("issue payload = %v", fake.issue)
	}
	// The existing label keeps its id; the type had no label, so one named
	// "bug" was created and its new id used.
	got, _ := json.Marshal(fake.issue["labels"])
	if string(got) != "[3,101]" {
		t.Errorf("label ids = %s, want [3,101]", got)
	}
	if strings.Join(fake.createdLabels, ",") != "bug" {
		t.Errorf("created labels = %v, want [bug]", fake.createdLabels)
	}
	for _, a := range fake.auth {
		if a != "token secret-token" {
			t.Errorf("Authorization = %q", a)
		}
	}
}

func TestForgejoCreateIssueFindsLabelOnSecondPage(t *testing.T) {
	fake := &forgejoFake{t: t, nextLabelID: 1000}
	for i := range forgejoLabelPageSize {
		fake.labels = append(fake.labels, forgejoLabel{ID: int64(i + 1), Name: "filler-" + string(rune('a'+i%26)) + string(rune('a'+i/26))})
	}
	fake.labels = append(fake.labels, forgejoLabel{ID: 777, Name: "user-feedback"})
	client := newForgejoTestClient(t, fake)

	if _, err := client.CreateIssue(context.Background(), "", "", "t", "b", "", []string{"user-feedback"}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	got, _ := json.Marshal(fake.issue["labels"])
	if string(got) != "[777]" {
		t.Errorf("label ids = %s, want [777]", got)
	}
	if len(fake.createdLabels) != 0 {
		t.Errorf("no label should be created, got %v", fake.createdLabels)
	}
}

func TestForgejoCreateIssueWithoutLabelsSkipsTheLookup(t *testing.T) {
	fake := &forgejoFake{t: t}
	client := newForgejoTestClient(t, fake)

	if _, err := client.CreateIssue(context.Background(), "", "", "t", "b", "", nil); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if _, has := fake.issue["labels"]; has {
		t.Errorf("payload must carry no labels field, got %v", fake.issue)
	}
	if len(fake.auth) != 1 {
		t.Errorf("expected one request (the issue), saw %d", len(fake.auth))
	}
}

func TestForgejoCreateIssueRequiresTitle(t *testing.T) {
	client := newForgejoTestClient(t, &forgejoFake{t: t})
	if _, err := client.CreateIssue(context.Background(), "", "", "", "b", "", nil); err == nil {
		t.Fatal("an empty title must be an error")
	}
}

func TestForgejoErrorCarriesServerMessageNotToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"token does not have required scope"}`))
	}))
	t.Cleanup(srv.Close)
	client, err := NewForgejoClient(ForgejoConfig{BaseURL: srv.URL, Token: "secret-token", Owner: "acme", Repo: "tracker"})
	if err != nil {
		t.Fatalf("NewForgejoClient: %v", err)
	}

	_, err = client.CreateIssue(context.Background(), "", "", "t", "b", "", nil)
	if err == nil {
		t.Fatal("a 403 must be an error")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "required scope") {
		t.Errorf("error should carry the status and the server's message: %v", err)
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Errorf("error must not contain the token: %v", err)
	}
}
