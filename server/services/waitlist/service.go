package waitlist

import (
	"context"
	"net/mail"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/email"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the WaitlistService RPC interface.
type Service struct {
	storage      *storage.ProtoSQLStorage
	emailService email.Service
	notifyEmail  string
}

// New creates a new waitlist service.
func New(sqlStorage *storage.ProtoSQLStorage, emailService email.Service, notifyEmail string) *Service {
	return &Service{
		storage:      sqlStorage,
		emailService: emailService,
		notifyEmail:  notifyEmail,
	}
}

// JoinWaitlist adds an email to the waitlist.
func (s *Service) JoinWaitlist(
	ctx context.Context,
	req *connect.Request[api.JoinWaitlistRequest],
) (*connect.Response[api.JoinWaitlistResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "JoinWaitlist",
	)

	emailAddr := req.Msg.Email

	// Validate email format
	if _, err := mail.ParseAddress(emailAddr); err != nil {
		logger.InfoContext(ctx, "invalid email format",
			"email", logging.MaskEmail(emailAddr),
		)
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	logger.InfoContext(ctx, "processing waitlist signup",
		"email", logging.MaskEmail(emailAddr),
	)

	// Check if email already exists
	existing, err := s.storage.QueryByField(ctx, "email", emailAddr, &models.WaitlistEntry{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query for existing entry",
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "JoinWaitlist", err)
	}

	if len(existing) > 0 {
		logger.InfoContext(ctx, "email already on waitlist",
			"email", logging.MaskEmail(emailAddr),
		)
		return connect.NewResponse(&api.JoinWaitlistResponse{
			Message:           "You're already on the list!",
			AlreadyRegistered: true,
		}), nil
	}

	// Insert new entry
	entry := &models.WaitlistEntry{
		Id:               uuid.New().String(),
		Email:            emailAddr,
		CreatedAtUnixSec: time.Now().Unix(),
		Name:             req.Msg.Name,
		ReferralSource:   req.Msg.ReferralSource,
		IntendedUse:      req.Msg.IntendedUse,
	}

	if _, err := s.storage.Insert(ctx, entry); err != nil {
		logger.ErrorContext(ctx, "failed to insert waitlist entry",
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "JoinWaitlist", err)
	}

	logger.InfoContext(ctx, "waitlist entry created",
		"email", logging.MaskEmail(emailAddr),
		"entry_id", entry.Id,
	)

	// Send welcome email (don't fail if email fails). The goroutine
	// detaches the request ctx, so capture the locale up-front and
	// re-inject it into the background ctx — otherwise the email
	// would always render in the default locale regardless of the
	// signup request's Accept-Language header.
	signupLocale := l10n.LocaleFromContext(ctx).String()
	logging.GoSafe(ctx, "send-waitlist-welcome", func() {
		emailCtx := l10n.WithLocale(context.Background(), signupLocale)
		if err := s.emailService.SendWaitlistWelcome(emailCtx, emailAddr); err != nil {
			logging.Default().Error("failed to send waitlist welcome email",
				"error", err,
				"email", logging.MaskEmail(emailAddr),
			)
		}
	})

	// Send notification to developer (don't fail if email fails)
	if s.notifyEmail != "" {
		details := email.WaitlistSignupDetails{
			Email:          emailAddr,
			Name:           entry.GetName(),
			ReferralSource: entry.GetReferralSource(),
			IntendedUse:    entry.GetIntendedUse(),
		}
		logging.GoSafe(ctx, "send-waitlist-notification", func() {
			emailCtx := context.Background()
			if err := s.emailService.SendWaitlistNotification(emailCtx, s.notifyEmail, details); err != nil {
				logging.Default().Error("failed to send waitlist notification",
					"error", err,
					"notify_email", logging.MaskEmail(s.notifyEmail),
				)
			}
		})
	}

	return connect.NewResponse(&api.JoinWaitlistResponse{
		Message:           "Welcome to the community!",
		AlreadyRegistered: false,
	}), nil
}

// Verify that Service implements the WaitlistServiceHandler interface.
var _ apiconnect.WaitlistServiceHandler = (*Service)(nil)
