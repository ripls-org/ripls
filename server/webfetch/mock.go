package webfetch

import "context"

// MockFetcher is a mock implementation of Fetcher for testing.
type MockFetcher struct {
	// FetchPageContentFunc is called when FetchPageContent is invoked.
	// Set this to control the mock's behavior.
	FetchPageContentFunc func(ctx context.Context, url string) (*PageContent, error)

	// FetchPageContentCalls records all calls to FetchPageContent.
	FetchPageContentCalls []string
}

// FetchPageContent implements Fetcher.
func (m *MockFetcher) FetchPageContent(ctx context.Context, url string) (*PageContent, error) {
	m.FetchPageContentCalls = append(m.FetchPageContentCalls, url)

	if m.FetchPageContentFunc != nil {
		return m.FetchPageContentFunc(ctx, url)
	}

	// Default: return empty content
	return &PageContent{URL: url}, nil
}

// Reset clears the recorded calls.
func (m *MockFetcher) Reset() {
	m.FetchPageContentCalls = nil
}
