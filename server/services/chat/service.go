package chat

import (
	"sync"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"

	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/storage"
)

// streamInfo tracks a stream channel and the associated user ID.
type streamInfo struct {
	userID string
	ch     chan *api.StreamMessagesResponse
}

// Service implements the ChatService RPC interface.
type Service struct {
	storage         *storage.ProtoSQLStorage
	chatConvStorage *storage.ChatConversationStorage
	bus             chat_event_bus.Publisher
	aiProvider      ai.Provider

	// In-memory stream registry for real-time message broadcasting.
	streamsMu sync.RWMutex
	streams   map[string][]streamInfo // conversationID → list of active streams

	// In-memory user presence tracking (foreground/background state).
	presenceMu       sync.RWMutex
	userInForeground map[string]bool // userID → isInForeground
}

// New creates a new chat service. bus is the chat event bus that the service
// publishes to; subscribers (stream broadcaster, push notifications) are
// registered on the bus in main.go.
func New(sqlStorage *storage.ProtoSQLStorage, bus chat_event_bus.Publisher) *Service {
	return &Service{
		storage:          sqlStorage,
		chatConvStorage:  storage.NewChatConversationStorage(sqlStorage),
		bus:              bus,
		streams:          make(map[string][]streamInfo),
		userInForeground: make(map[string]bool),
	}
}

// SetAIProvider sets the AI provider for conversation summarization.
func (s *Service) SetAIProvider(provider ai.Provider) {
	s.aiProvider = provider
}

// HasActiveStream is the exported stream-presence hook wired into the chat
// notification subscriber via SetStreamChecker in main.go.
func (s *Service) HasActiveStream(conversationID, userID string) bool {
	return s.hasActiveStream(conversationID, userID)
}

// IsUserInForeground is the exported foreground-presence hook wired into the
// chat notification subscriber via SetForegroundChecker in main.go.
func (s *Service) IsUserInForeground(userID string) bool {
	return s.isUserInForeground(userID)
}

// Verify that Service implements the ChatServiceHandler interface.
var _ apiconnect.ChatServiceHandler = (*Service)(nil)
