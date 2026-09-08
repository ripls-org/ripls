package email

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"strconv"
	texttemplate "text/template"
	"time"

	"github.com/mailgun/mailgun-go/v5"
	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
)

// Service defines the interface for sending emails.
type Service interface {
	// SendPasswordReset renders and sends the password-reset email
	// in the recipient's preferred language. preferredLanguage is
	// the recipient User's stored preferred_language (BCP-47 tag);
	// pass "" when no User record exists or the field is unset, in
	// which case the locale falls back to the request's
	// Accept-Language and then to English.
	SendPasswordReset(ctx context.Context, toEmail, resetURL, preferredLanguage string) error
	// SendEmailCode renders and sends the one-time sign-in code that
	// proves the recipient can receive mail at toEmail. The recipient
	// usually has no User row yet (the same code serves sign-up and
	// sign-in), so preferredLanguage is often "" and the locale falls
	// back to the request's Accept-Language and then to English.
	//
	// This send is on the interactive critical path: unlike
	// SendPasswordReset, whose failure is swallowed so it cannot become
	// an account-existence oracle, a failure here must be surfaced to
	// the caller. A silently dropped code leaves the person waiting at a
	// code-entry screen forever. Implementations must never log the code
	// itself, at any level.
	SendEmailCode(ctx context.Context, toEmail, code string, expiresIn time.Duration, preferredLanguage string) error
	// SendWaitlistWelcome sends a welcome email to a newly-waitlisted
	// address. The recipient has no User row yet, so locale comes
	// from the request's Accept-Language (via the ctx) and falls
	// back to English. Callers that detach the request ctx to
	// background the send must propagate the locale via
	// l10n.WithLocale before spawning the goroutine.
	SendWaitlistWelcome(ctx context.Context, toEmail string) error
	SendWaitlistNotification(ctx context.Context, notifyEmail string, details WaitlistSignupDetails) error
	// SendActivityDigest sends the daily activity digest to the given
	// recipient. The body of the email intentionally contains raw
	// user-identifying values (names, emails); log lines emitted by the
	// implementation must mask the recipient via logging.MaskEmail.
	SendActivityDigest(ctx context.Context, toEmail string, in ActivityDigestInput) error
	// SendNotification sends an off-app notification email — the email
	// channel for a recipient who has no active app device. Title and Body
	// are already localized (rendered upstream from models.Notification in
	// the recipient's locale); this only wraps them in the branded shell
	// with a CAN-SPAM footer. in.PreferredLanguage localizes the shell.
	SendNotification(ctx context.Context, in NotificationEmailInput) error
	// CheckHealth verifies the email service is accessible and returns status information.
	CheckHealth(ctx context.Context) ([]*health.Status, error)
}

// NotificationEmailInput carries an already-localized off-app notification for
// a deviceless recipient. Title/Body come straight from models.Notification
// (rendered upstream in the recipient's locale); the email only wraps them in
// the branded shell + CAN-SPAM footer. ActionURL and UnsubscribeURL are
// optional and omitted from the rendered body when empty.
type NotificationEmailInput struct {
	ToEmail           string
	PreferredLanguage string // recipient User.preferred_language; "" → ctx/default
	Title             string
	Body              string
	ActionURL         string // optional CTA target (the /go deep link)
	CTALabel          string // optional CTA button label (e.g. "RSVP"); "" → "Open Ripls"
	UnsubscribeURL    string // one-click unsubscribe (CAN-SPAM)
	// ColdInvite marks a first-contact invite to someone with no Ripls account
	// yet (the email.InviteSender path), which changes the footer's
	// "why am I getting this" line. False for activity notifications to existing
	// accounts.
	ColdInvite bool

	// HeroImageURL is the src for the email's hero image: an http(s) URL (used by
	// the examples/preview tooling) or a "cid:<filename>" reference into
	// InlineImages for production mail. Empty renders the card with no photo.
	HeroImageURL string
	// InlineImages are embedded in the message via multipart/related and
	// referenced from the HTML by "cid:<Filename>". CID embedding renders
	// whenever the mail is opened — unlike a hotlinked presigned bucket URL,
	// which expires before a recipient may open a days-old email.
	InlineImages []InlineImage
	// Summary is the optional rich entity card (event/gear/request details),
	// the email analogue of the SSR landing-page summary. Nil → plain layout.
	Summary *ItemSummary
}

// InlineImage is an image embedded in the email body via a multipart/related
// CID part. The CID equals Filename, so the HTML references it as
// "cid:<Filename>".
type InlineImage struct {
	Filename string
	Data     []byte
}

// ItemSummary is the entity-specific detail rendered in the off-app email's
// themed card — the email analogue of the SSR landing-page summary. With a
// summary present the card heading becomes the entity Name (like a landing
// page), and the message sentence (NotificationEmailInput.Title, also the
// subject) reads as the action below it — so subject and heading differ. All
// fields are optional; the template omits empty ones.
type ItemSummary struct {
	Eyebrow     string // small label above the title, e.g. "New event" / "Free to a good home"
	Name        string // the entity name — the big card heading ("Backyard BBQ")
	OwnerLine   string // "Hosted by Sam" / "Shared by Sam" / "Requested by Sam"
	Date        string // event date line, e.g. "June 27"
	Going       string // RSVP headcount line for events, e.g. "5 going"
	Description string // truncated entity/event description
}

// Custom variables stamped on every outbound message so the delivery webhook
// can attribute what it receives. Mailgun echoes them back verbatim under
// `event-data.user-variables`, which is what makes true accepted→delivered
// latency measurable without a correlation table: the send time travels with
// the message instead of being looked up.
const (
	// deliveryVarEmailType names the flow (email_code, off_app_notification, …)
	// so a sign-in code's deliverability can be read separately from a digest's.
	// It is the label that makes the metric actionable — a slow digest is a
	// nuisance, a slow sign-in code keeps someone out.
	deliveryVarEmailType = "ripls_email_type"
	// deliveryVarSentAtMs is the accept time in Unix milliseconds.
	deliveryVarSentAtMs = "ripls_sent_at_ms"
	// deliveryVarEnv identifies the environment that sent the message.
	//
	// Necessary because Mailgun webhooks are configured per *domain*, and dev
	// and prod share one — `ripls.app` is the only real domain on the account.
	// Every registered URL therefore receives every environment's events. The
	// webhook drops what it did not send, which keeps a dev simulation run
	// (thousands of addresses) from swamping production's delivery metrics.
	deliveryVarEnv = "ripls_env"
)

// prepareOutbound applies what every outgoing message needs: the inline logo,
// and the delivery attribution the webhook reads back.
//
// One function rather than two calls per send site, so a new email flow cannot
// pick up the metrics and forget the logo (or the reverse) — the failure mode
// when this was two separate steps was exactly that drift.
//
// Best-effort throughout: neither a missing logo nor a missing variable is
// worth failing a send the recipient is waiting on. Failures are logged.
func (s *MailgunService) prepareOutbound(ctx context.Context, m *mailgun.PlainMessage, emailType string) {
	// Referenced from the templates as <img src="cid:logo.png">.
	m.AddReaderInline("logo.png", io.NopCloser(bytes.NewReader(logoPNG)))
	s.stampDeliveryVariables(ctx, m, emailType)
}

// stampDeliveryVariables attaches the attribution the delivery webhook reads
// back. Best-effort: a stamping failure must never block the send, since the
// message itself is what the recipient needs and the variables only serve
// observability. Failures are logged rather than returned.
func (s *MailgunService) stampDeliveryVariables(ctx context.Context, m *mailgun.PlainMessage, emailType string) {
	set := func(k, v string) {
		if err := m.AddVariable(k, v); err != nil {
			logging.LoggerWithContext(ctx).WarnContext(ctx,
				"failed to stamp delivery variable; delivery metrics will miss this send",
				"variable", k, "email_type", emailType, "error", err,
			)
		}
	}
	set(deliveryVarEmailType, emailType)
	set(deliveryVarSentAtMs, strconv.FormatInt(time.Now().UnixMilli(), 10))
	if s.environmentTag != "" {
		set(deliveryVarEnv, s.environmentTag)
	}
}

// App distribution links are per-deployment (a fork must not send its users to
// our store listings), so they arrive via SetStoreLinks rather than being
// compiled in. Unset links omit the download CTA entirely — see the
// {{if or .PlayStoreURL .AppStoreURL}} guard in the notification templates.

// WaitlistSignupDetails contains the details of a waitlist signup for notification emails.
type WaitlistSignupDetails struct {
	Email          string
	Name           string
	ReferralSource string
	IntendedUse    string
}

//go:embed password_reset_template_en.html
var passwordResetTemplateENHTML string

//go:embed password_reset_template_en.txt
var passwordResetTemplateENText string

//go:embed password_reset_template_es.html
var passwordResetTemplateESHTML string

//go:embed password_reset_template_es.txt
var passwordResetTemplateESText string

// logoPNG is the Ripls mark, embedded in every outgoing email as an inline
// attachment rather than hotlinked.
//
// A remote <img> is a read receipt: the recipient's client fetches it on open,
// telling the server who opened what and when. Embedding removes that callback
// entirely — nobody is pinged when someone reads their mail. It also makes the
// logo render when a client blocks remote content, which most now do by
// default, so the alternative was a broken-image box on the one email people
// must act on.
//
// Palette-reduced to 192px (~10KB, from a 60KB 309px source): it renders at
// 40–80px, and this rides on every message we send.
//
//go:embed logo.png
var logoPNG []byte

//go:embed email_code_template_en.html
var emailCodeTemplateENHTML string

//go:embed email_code_template_en.txt
var emailCodeTemplateENText string

//go:embed email_code_template_es.html
var emailCodeTemplateESHTML string

//go:embed email_code_template_es.txt
var emailCodeTemplateESText string

//go:embed waitlist_welcome_template_en.html
var waitlistWelcomeTemplateENHTML string

//go:embed waitlist_welcome_template_en.txt
var waitlistWelcomeTemplateENText string

//go:embed waitlist_welcome_template_es.html
var waitlistWelcomeTemplateESHTML string

//go:embed waitlist_welcome_template_es.txt
var waitlistWelcomeTemplateESText string

//go:embed waitlist_notification_template.html
var waitlistNotificationTemplateHTML string

//go:embed waitlist_notification_template.txt
var waitlistNotificationTemplateText string

//go:embed notification_template_en.html
var notificationTemplateENHTML string

//go:embed notification_template_en.txt
var notificationTemplateENText string

//go:embed notification_template_es.html
var notificationTemplateESHTML string

//go:embed notification_template_es.txt
var notificationTemplateESText string

// localizedTemplates pairs the HTML and plain-text renderings of a
// single email flow across every supported locale. The render path
// looks up the tag with [pickLocalizedTemplates] and falls back to
// [l10n.DefaultTag] when an entry is missing — matching the
// localizer's fallback behavior for subject strings.
type localizedTemplates struct {
	html map[language.Tag]*template.Template
	// text is parsed with text/template (NOT html/template) so the text/plain
	// MIME part renders literal apostrophes and ampersands rather than HTML
	// entities (&#39; / &amp;). html-escaping is correct only for the html part.
	text map[language.Tag]*texttemplate.Template
}

func (lt localizedTemplates) pick(tag language.Tag) (*template.Template, *texttemplate.Template) {
	htmlTmpl, ok := lt.html[tag]
	if !ok {
		htmlTmpl = lt.html[l10n.DefaultTag]
	}
	textTmpl, ok := lt.text[tag]
	if !ok {
		textTmpl = lt.text[l10n.DefaultTag]
	}
	return htmlTmpl, textTmpl
}

// MailgunService handles sending emails via Mailgun.
type MailgunService struct {
	mg                               *mailgun.Client
	domain                           string
	fromAddress                      string
	postalAddress                    string
	legalEntityName                  string
	environmentTag                   string
	playStoreURL                     string
	appStoreURL                      string
	passwordReset                    localizedTemplates
	emailCode                        localizedTemplates
	waitlistWelcome                  localizedTemplates
	notification                     localizedTemplates
	waitlistNotificationHTMLTemplate *template.Template
	waitlistNotificationTextTemplate *template.Template
	activityDigestHTMLTemplate       *template.Template
	activityDigestTextTemplate       *texttemplate.Template
}

// SetPostalAddress sets the CAN-SPAM physical postal address rendered in the
// off-app notification email footer (the single source of truth for the line,
// shared across locales — a postal address is not translated). Required by
// CAN-SPAM before the off-app email channel is enabled; when empty the footer
// omits the address line entirely, which is why the channel ships flag-off
// until this is set. Set once at startup from non-secret config.
func (s *MailgunService) SetPostalAddress(addr string) { s.postalAddress = addr }

// SetLegalEntityName sets the operator name signed at the foot of the waitlist
// welcome email. Per-deployment: signing a fork's mail with another operator's
// legal name is worse than signing it with nothing, so an empty value drops the
// line entirely rather than falling back to a default (#2953). Set once at
// startup.
func (s *MailgunService) SetLegalEntityName(name string) { s.legalEntityName = name }

// SetStoreLinks sets the app-distribution URLs shown in the notification
// footer's download CTA. These are per-deployment: a fork's users must not be
// sent to another operator's store listing, so an unset link is omitted rather
// than defaulted. Both empty removes the CTA. Set once at startup.
func (s *MailgunService) SetStoreLinks(appStoreURL, playStoreURL string) {
	s.appStoreURL, s.playStoreURL = appStoreURL, playStoreURL
}

// SetEnvironmentTag records which environment this server is, stamped onto
// every outbound message so the delivery webhook can drop events another
// environment sent. Fed from the per-environment invite hostname, which is the
// value that already differs between dev and prod. Empty disables the stamp,
// in which case the webhook cannot attribute the message and skips it.
func (s *MailgunService) SetEnvironmentTag(tag string) { s.environmentTag = tag }

// NewMailgunService creates a new Mailgun email service.
func NewMailgunService(domain, apiKey, fromAddress string) (*MailgunService, error) {
	mg := mailgun.NewMailgun(apiKey)

	passwordReset, err := buildLocalizedTemplates("passwordReset", map[language.Tag]localeSource{
		language.English: {html: passwordResetTemplateENHTML, text: passwordResetTemplateENText},
		language.Spanish: {html: passwordResetTemplateESHTML, text: passwordResetTemplateESText},
	})
	if err != nil {
		return nil, err
	}

	emailCode, err := buildLocalizedTemplates("emailCode", map[language.Tag]localeSource{
		language.English: {html: emailCodeTemplateENHTML, text: emailCodeTemplateENText},
		language.Spanish: {html: emailCodeTemplateESHTML, text: emailCodeTemplateESText},
	})
	if err != nil {
		return nil, err
	}

	waitlistWelcome, err := buildLocalizedTemplates("waitlistWelcome", map[language.Tag]localeSource{
		language.English: {html: waitlistWelcomeTemplateENHTML, text: waitlistWelcomeTemplateENText},
		language.Spanish: {html: waitlistWelcomeTemplateESHTML, text: waitlistWelcomeTemplateESText},
	})
	if err != nil {
		return nil, err
	}

	notification, err := buildLocalizedTemplates("notification", map[language.Tag]localeSource{
		language.English: {html: notificationTemplateENHTML, text: notificationTemplateENText},
		language.Spanish: {html: notificationTemplateESHTML, text: notificationTemplateESText},
	})
	if err != nil {
		return nil, err
	}

	waitlistNotificationHTML, err := parseEmailHTML("waitlistNotificationHTML", waitlistNotificationTemplateHTML)
	if err != nil {
		return nil, fmt.Errorf("failed to parse waitlist notification HTML template: %w", err)
	}
	waitlistNotificationText, err := template.New("waitlistNotificationText").Parse(waitlistNotificationTemplateText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse waitlist notification text template: %w", err)
	}

	activityDigestHTML, err := parseEmailHTML("activityDigestHTML", activityDigestTemplateHTML)
	if err != nil {
		return nil, fmt.Errorf("failed to parse activity digest HTML template: %w", err)
	}
	activityDigestText, err := texttemplate.New("activityDigestText").Parse(activityDigestTemplateText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse activity digest text template: %w", err)
	}

	return &MailgunService{
		mg:                               mg,
		domain:                           domain,
		fromAddress:                      fromAddress,
		passwordReset:                    passwordReset,
		emailCode:                        emailCode,
		waitlistWelcome:                  waitlistWelcome,
		notification:                     notification,
		waitlistNotificationHTMLTemplate: waitlistNotificationHTML,
		waitlistNotificationTextTemplate: waitlistNotificationText,
		activityDigestHTMLTemplate:       activityDigestHTML,
		activityDigestTextTemplate:       activityDigestText,
	}, nil
}

type localeSource struct {
	html string
	text string
}

func buildLocalizedTemplates(name string, sources map[language.Tag]localeSource) (localizedTemplates, error) {
	lt := localizedTemplates{
		html: make(map[language.Tag]*template.Template, len(sources)),
		text: make(map[language.Tag]*texttemplate.Template, len(sources)),
	}
	for tag, src := range sources {
		htmlTmpl, err := parseEmailHTML(name+"HTML_"+tag.String(), src.html)
		if err != nil {
			return localizedTemplates{}, fmt.Errorf("parse %s HTML (%s): %w", name, tag, err)
		}
		textTmpl, err := texttemplate.New(name + "Text_" + tag.String()).Parse(src.text)
		if err != nil {
			return localizedTemplates{}, fmt.Errorf("parse %s text (%s): %w", name, tag, err)
		}
		lt.html[tag] = htmlTmpl
		lt.text[tag] = textTmpl
	}
	return lt, nil
}

// RenderPasswordReset renders the password-reset email body in the
// locale resolved from preferredLanguage and ctx without invoking
// Mailgun. Exposed for tests and offline preview tooling; bytes are
// identical to what SendPasswordReset would put on the wire.
func (s *MailgunService) RenderPasswordReset(ctx context.Context, resetURL, preferredLanguage string) (htmlBody, textBody, subject string, tag language.Tag, err error) {
	tag = localeForRecipient(ctx, preferredLanguage)
	loc, locErr := l10n.NewLocalizer(tag)
	if locErr != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"failed to build email localizer; rendering in default locale",
			"error", locErr,
			"locale", tag.String(),
		)
	}
	subject = loc.T(ctx, "email.password_reset.subject", nil)

	data := struct {
		ResetURL string
		Colors   templateColors
	}{ResetURL: resetURL, Colors: emailPalette()}

	htmlTmpl, textTmpl := s.passwordReset.pick(tag)
	var htmlBuf, textBuf bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", "", tag, fmt.Errorf("execute password reset HTML template: %w", err)
	}
	if err := textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", "", tag, fmt.Errorf("execute password reset text template: %w", err)
	}
	return htmlBuf.String(), textBuf.String(), subject, tag, nil
}

// RenderEmailCode renders the sign-in-code email body in the locale
// resolved from preferredLanguage and ctx without invoking Mailgun.
// Exposed for tests and offline preview tooling; bytes are identical to
// what SendEmailCode would put on the wire.
func (s *MailgunService) RenderEmailCode(ctx context.Context, code string, expiresIn time.Duration, preferredLanguage string) (htmlBody, textBody, subject string, tag language.Tag, err error) {
	tag = localeForRecipient(ctx, preferredLanguage)
	loc, locErr := l10n.NewLocalizer(tag)
	if locErr != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"failed to build email localizer; rendering in default locale",
			"error", locErr,
			"locale", tag.String(),
		)
	}
	subject = loc.T(ctx, "email.email_code.subject", nil)

	data := struct {
		Code             string
		ExpiresInMinutes int
		Colors           templateColors
	}{
		Code:             code,
		ExpiresInMinutes: int(expiresIn.Minutes()),
		Colors:           emailPalette(),
	}

	htmlTmpl, textTmpl := s.emailCode.pick(tag)
	var htmlBuf, textBuf bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", "", tag, fmt.Errorf("execute email code HTML template: %w", err)
	}
	if err := textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", "", tag, fmt.Errorf("execute email code text template: %w", err)
	}
	return htmlBuf.String(), textBuf.String(), subject, tag, nil
}

// SendEmailCode sends the one-time sign-in code in the recipient's locale.
// The code is never logged — only the masked recipient and the outcome.
func (s *MailgunService) SendEmailCode(ctx context.Context, toEmail, code string, expiresIn time.Duration, preferredLanguage string) error {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx)

	htmlBody, textBody, subject, tag, err := s.RenderEmailCode(ctx, code, expiresIn, preferredLanguage)
	if err != nil {
		logger.Error("failed to render email code",
			"error", err,
			"recipient_email", logging.MaskEmail(toEmail),
			"email_type", "email_code",
			"locale", tag.String(),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return err
	}

	logger.Info("sending email",
		"recipient_email", logging.MaskEmail(toEmail),
		"email_type", "email_code",
		"locale", tag.String(),
		"recipient_count", 1,
	)

	// Skip delivery to reserved test domains. The code is already stored and
	// the dev-mode echo already carries it, so every test path still works —
	// this only drops a network call that could not succeed. See
	// IsUndeliverableTestAddress for why the bounces matter.
	if IsUndeliverableTestAddress(toEmail) {
		logger.Info("skipping send to a reserved test domain",
			"recipient_email", logging.MaskEmail(toEmail),
			"email_type", "email_code",
		)
		return nil
	}

	message := mailgun.NewMessage(s.domain, s.fromAddress, subject, textBody, toEmail)
	message.SetHTML(htmlBody)
	s.prepareOutbound(ctx, message, "email_code")

	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	if _, err := s.mg.Send(ctx, message); err != nil {
		logger.Error("failed to send email via mailgun",
			"error", err,
			"recipient_email", logging.MaskEmail(toEmail),
			"email_type", "email_code",
			"locale", tag.String(),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("failed to send email via Mailgun: %w", err)
	}

	logger.Info("email sent successfully",
		"recipient_email", logging.MaskEmail(toEmail),
		"email_type", "email_code",
		"locale", tag.String(),
		"recipient_count", 1,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return nil
}

// SendPasswordReset sends a password reset email in the recipient's locale.
func (s *MailgunService) SendPasswordReset(ctx context.Context, toEmail, resetURL, preferredLanguage string) error {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx)

	htmlBody, textBody, subject, tag, err := s.RenderPasswordReset(ctx, resetURL, preferredLanguage)
	if err != nil {
		logger.Error("failed to render password reset email",
			"error", err,
			"recipient_email", logging.MaskEmail(toEmail),
			"email_type", "password_reset",
			"locale", tag.String(),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return err
	}

	logger.Info("sending email",
		"recipient_email", logging.MaskEmail(toEmail),
		"email_type", "password_reset",
		"locale", tag.String(),
		"recipient_count", 1,
	)

	if IsUndeliverableTestAddress(toEmail) {
		logger.Info("skipping send to a reserved test domain",
			"recipient_email", logging.MaskEmail(toEmail),
			"email_type", "password_reset",
		)
		return nil
	}

	message := mailgun.NewMessage(s.domain, s.fromAddress, subject, textBody, toEmail)
	message.SetHTML(htmlBody)
	s.prepareOutbound(ctx, message, "password_reset")

	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	if _, err := s.mg.Send(ctx, message); err != nil {
		logger.Error("failed to send email via mailgun",
			"error", err,
			"recipient_email", logging.MaskEmail(toEmail),
			"email_type", "password_reset",
			"locale", tag.String(),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("failed to send email via Mailgun: %w", err)
	}

	logger.Info("email sent successfully",
		"recipient_email", logging.MaskEmail(toEmail),
		"email_type", "password_reset",
		"locale", tag.String(),
		"recipient_count", 1,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return nil
}

// RenderNotification renders the off-app notification email body in the
// locale resolved from in.PreferredLanguage and ctx without invoking Mailgun.
// Title/Body are already localized upstream; this only wraps them. Exposed for
// tests and offline preview tooling.
func (s *MailgunService) RenderNotification(ctx context.Context, in NotificationEmailInput) (htmlBody, textBody, subject string, tag language.Tag, err error) {
	tag = localeForRecipient(ctx, in.PreferredLanguage)
	// The subject is the already-localized notification title.
	subject = in.Title

	data := struct {
		Title          string
		Body           string
		ActionURL      string
		CTALabel       string
		UnsubscribeURL string
		PostalAddress  string
		ColdInvite     bool
		PlayStoreURL   string
		AppStoreURL    string
		HeroImageURL   template.URL
		Summary        *ItemSummary
		Colors         templateColors
	}{
		Title:          in.Title,
		Body:           in.Body,
		ActionURL:      in.ActionURL,
		CTALabel:       in.CTALabel,
		UnsubscribeURL: in.UnsubscribeURL,
		PostalAddress:  s.postalAddress,
		ColdInvite:     in.ColdInvite,
		PlayStoreURL:   s.playStoreURL,
		AppStoreURL:    s.appStoreURL,
		// Trusted: HeroImageURL is a "cid:" reference or our own image URL, never
		// user input, so bypass html/template's URL-scheme sanitizer (which would
		// otherwise rewrite "cid:" to "#ZgotmplZ").
		//nolint:gosec // G203: the value is built by notifications.buildEmailExtras as
		// "cid:"+<filename we generate> or one of our own image URLs — never user
		// input. The template.URL bypass exists precisely so html/template stops
		// rewriting "cid:" to "#ZgotmplZ".
		HeroImageURL: template.URL(in.HeroImageURL),
		Summary:      in.Summary,
		Colors:       emailPalette(),
	}

	htmlTmpl, textTmpl := s.notification.pick(tag)
	var htmlBuf, textBuf bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", "", tag, fmt.Errorf("execute notification HTML template: %w", err)
	}
	if err := textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", "", tag, fmt.Errorf("execute notification text template: %w", err)
	}
	return htmlBuf.String(), textBuf.String(), subject, tag, nil
}

// SendNotification sends an off-app notification email in the recipient's locale.
func (s *MailgunService) SendNotification(ctx context.Context, in NotificationEmailInput) error {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx)

	htmlBody, textBody, subject, tag, err := s.RenderNotification(ctx, in)
	if err != nil {
		logger.Error("failed to render notification email",
			"error", err,
			"recipient_email", logging.MaskEmail(in.ToEmail),
			"email_type", "off_app_notification",
			"locale", tag.String(),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return err
	}

	logger.Info("sending email",
		"recipient_email", logging.MaskEmail(in.ToEmail),
		"email_type", "off_app_notification",
		"locale", tag.String(),
		"recipient_count", 1,
	)

	if IsUndeliverableTestAddress(in.ToEmail) {
		logger.Info("skipping send to a reserved test domain",
			"recipient_email", logging.MaskEmail(in.ToEmail),
			"email_type", "off_app_notification",
		)
		return nil
	}

	message := mailgun.NewMessage(s.domain, s.fromAddress, subject, textBody, in.ToEmail)
	message.SetHTML(htmlBody)
	s.prepareOutbound(ctx, message, "off_app_notification")
	// Embed hero/summary images inline (multipart/related). The HTML references
	// each as "cid:<Filename>", so it renders whenever the mail is opened.
	for _, img := range in.InlineImages {
		message.AddReaderInline(img.Filename, io.NopCloser(bytes.NewReader(img.Data)))
	}

	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	if _, err := s.mg.Send(ctx, message); err != nil {
		logger.Error("failed to send email via mailgun",
			"error", err,
			"recipient_email", logging.MaskEmail(in.ToEmail),
			"email_type", "off_app_notification",
			"locale", tag.String(),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("failed to send email via Mailgun: %w", err)
	}

	logger.Info("email sent successfully",
		"recipient_email", logging.MaskEmail(in.ToEmail),
		"email_type", "off_app_notification",
		"locale", tag.String(),
		"recipient_count", 1,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return nil
}

// RenderWaitlistWelcome renders the waitlist-welcome body in the
// locale resolved from ctx (Accept-Language) without invoking
// Mailgun. Exposed for tests and offline preview tooling.
func (s *MailgunService) RenderWaitlistWelcome(ctx context.Context) (htmlBody, textBody, subject string, tag language.Tag, err error) {
	// No User row exists yet for a waitlist signup, so the only
	// available locale signal is the request ctx (Accept-Language
	// captured by the AcceptLanguage middleware, propagated into the
	// background ctx by the caller via l10n.WithLocale).
	tag = localeForRecipient(ctx, "")
	loc, locErr := l10n.NewLocalizer(tag)
	if locErr != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"failed to build email localizer; rendering in default locale",
			"error", locErr,
			"locale", tag.String(),
		)
	}
	subject = loc.T(ctx, "email.waitlist_welcome.subject", nil)

	data := struct {
		Colors          templateColors
		LegalEntityName string
	}{Colors: emailPalette(), LegalEntityName: s.legalEntityName}
	htmlTmpl, textTmpl := s.waitlistWelcome.pick(tag)
	var htmlBuf, textBuf bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", "", tag, fmt.Errorf("execute waitlist welcome HTML template: %w", err)
	}
	if err := textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", "", tag, fmt.Errorf("execute waitlist welcome text template: %w", err)
	}
	return htmlBuf.String(), textBuf.String(), subject, tag, nil
}

// SendWaitlistWelcome sends a welcome email to a new waitlist signup.
func (s *MailgunService) SendWaitlistWelcome(ctx context.Context, toEmail string) error {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx)

	htmlBody, textBody, subject, tag, err := s.RenderWaitlistWelcome(ctx)
	if err != nil {
		logger.Error("failed to render waitlist welcome email",
			"error", err,
			"recipient_email", logging.MaskEmail(toEmail),
			"locale", tag.String(),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return err
	}

	logger.Info("sending waitlist welcome email",
		"recipient_email", logging.MaskEmail(toEmail),
		"email_type", "waitlist_welcome",
		"locale", tag.String(),
	)

	if IsUndeliverableTestAddress(toEmail) {
		logger.Info("skipping send to a reserved test domain",
			"recipient_email", logging.MaskEmail(toEmail),
			"email_type", "waitlist_welcome",
		)
		return nil
	}

	message := mailgun.NewMessage(s.domain, s.fromAddress, subject, textBody, toEmail)
	message.SetHTML(htmlBody)
	s.prepareOutbound(ctx, message, "waitlist_welcome")

	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	if _, err := s.mg.Send(ctx, message); err != nil {
		logger.Error("failed to send waitlist welcome email",
			"error", err,
			"recipient_email", logging.MaskEmail(toEmail),
			"locale", tag.String(),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("failed to send waitlist welcome email: %w", err)
	}

	logger.Info("waitlist welcome email sent",
		"recipient_email", logging.MaskEmail(toEmail),
		"locale", tag.String(),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return nil
}

// SendWaitlistNotification notifies the developer of a new waitlist signup.
// Admin-only email (single configured developer recipient), intentionally
// English-only — not localized.
func (s *MailgunService) SendWaitlistNotification(ctx context.Context, notifyEmail string, details WaitlistSignupDetails) error {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx)

	logger.Info("sending waitlist notification",
		"signup_email", logging.MaskEmail(details.Email),
		"notify_email", logging.MaskEmail(notifyEmail),
		"email_type", "waitlist_notification",
	)

	subject := fmt.Sprintf("New Ripls waitlist signup: %s", details.Email)

	// Prepare template data
	data := struct {
		SignupEmail    string
		Timestamp      string
		Name           string
		ReferralSource string
		IntendedUse    string
		Colors         templateColors
	}{
		SignupEmail:    details.Email,
		Timestamp:      time.Now().Format("2006-01-02 15:04:05 MST"),
		Name:           details.Name,
		ReferralSource: details.ReferralSource,
		IntendedUse:    details.IntendedUse,
		Colors:         emailPalette(),
	}

	// Execute HTML template
	var htmlBuffer bytes.Buffer
	if err := s.waitlistNotificationHTMLTemplate.Execute(&htmlBuffer, data); err != nil {
		logger.Error("failed to execute waitlist notification HTML template",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("failed to execute waitlist notification HTML template: %w", err)
	}
	htmlBody := htmlBuffer.String()

	// Execute text template
	var textBuffer bytes.Buffer
	if err := s.waitlistNotificationTextTemplate.Execute(&textBuffer, data); err != nil {
		logger.Error("failed to execute waitlist notification text template",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("failed to execute waitlist notification text template: %w", err)
	}
	textBody := textBuffer.String()

	message := mailgun.NewMessage(
		s.domain,
		s.fromAddress,
		subject,
		textBody,
		notifyEmail,
	)
	s.prepareOutbound(ctx, message, "waitlist_notification")
	message.SetHTML(htmlBody)

	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	_, err := s.mg.Send(ctx, message)
	if err != nil {
		logger.Error("failed to send waitlist notification",
			"error", err,
			"notify_email", logging.MaskEmail(notifyEmail),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("failed to send waitlist notification: %w", err)
	}

	logger.Info("waitlist notification sent",
		"notify_email", logging.MaskEmail(notifyEmail),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return nil
}

// CheckHealth verifies Mailgun API connectivity by fetching domain info.
func (s *MailgunService) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()

	status := &health.Status{
		Name:    "email",
		Backend: "mailgun",
		Metadata: map[string]string{
			"domain": s.domain,
		},
	}

	_, err := s.mg.GetDomain(ctx, s.domain, nil)
	status.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		status.Error = err.Error()
	}

	return []*health.Status{status}, nil
}
