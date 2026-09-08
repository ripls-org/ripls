// Inbound SMS webhook — persist-only opt-out sync.
//
// Twilio's Messaging Service opt-out management owns the carrier-compliant
// STOP/HELP/START auto-replies and the authoritative opt-out list. This handler
// is a secondary mirror: it records the same state into our smsoptout store
// (so the send path can suppress before ever calling Twilio) and returns a bare
// <Response/> — it never sends its own reply, to avoid double-texting the
// recipient. The request is authenticated by its X-Twilio-Signature (HMAC-SHA1
// with the Twilio auth token); an invalid signature is rejected so a forged POST
// cannot toggle anyone's stored preference.

package web

import (
	"encoding/xml"
	"net/http"
	"strings"

	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/sms"
	"go.ripls.org/ripls/server/smsoptout"
)

// smsWebhookPath is the fixed public path Twilio is configured to POST to. It
// is reconstructed (with the service hostname) into the URL that the inbound
// signature is verified against.
const smsWebhookPath = "/sms/webhook"

// optOutAction is the resolved effect of an inbound keyword on our local store.
type optOutAction struct {
	keyword string // metric label: "stop", "start", "help"
	optOut  *bool  // non-nil sets opt-out state; nil leaves it unchanged
}

func boolPtr(b bool) *bool { return &b }

// classifyKeyword maps an inbound keyword to its effect on our store. It honors
// Twilio's opt-out-management OptOutType field when present, else the first word
// of the message body, against the standard CTIA keyword sets.
func classifyKeyword(optOutType, body string) (optOutAction, bool) {
	kw := strings.ToUpper(strings.TrimSpace(optOutType))
	if kw == "" {
		kw = strings.ToUpper(strings.TrimSpace(firstWord(body)))
	}
	switch kw {
	case "STOP", "STOPALL", "UNSUBSCRIBE", "CANCEL", "END", "QUIT":
		return optOutAction{keyword: "stop", optOut: boolPtr(true)}, true
	case "START", "YES", "UNSTOP":
		return optOutAction{keyword: "start", optOut: boolPtr(false)}, true
	case "HELP", "INFO":
		return optOutAction{keyword: "help"}, true
	default:
		return optOutAction{}, false
	}
}

func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// HandleSMSWebhook syncs our local opt-out store from an inbound Twilio keyword.
// Wired to POST /sms/webhook. It is persist-only: it records STOP/START into
// smsoptout and counts the keyword, then acknowledges with an empty TwiML
// response so Twilio's own opt-out management is the sole sender of the
// compliant reply. Always returns 200 once authenticated so Twilio does not
// retry; an unrecognized keyword is a no-op.
func (s *Service) HandleSMSWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.LoggerWithContext(ctx).With("operation", "SMSWebhook")

	if err := r.ParseForm(); err != nil {
		logger.InfoContext(ctx, "sms webhook: malformed form", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	publicURL := "https://" + s.hostname + smsWebhookPath
	if !sms.ValidateSignature(s.smsAuthToken, publicURL, r.PostForm, r.Header.Get("X-Twilio-Signature")) {
		logger.WarnContext(ctx, "sms webhook: invalid signature; rejecting")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if s.smsAuthToken == "" {
		logger.WarnContext(ctx, "sms webhook: signature verification disabled (no auth token configured)")
	}

	from := r.PostForm.Get("From")
	action, recognized := classifyKeyword(r.PostForm.Get("OptOutType"), r.PostForm.Get("Body"))
	if !recognized {
		s.writeEmptyTwiML(w)
		return
	}

	logger = logger.With("recipient_phone", logging.MaskPhone(from), "keyword", action.keyword)

	if action.optOut != nil && from != "" {
		var err error
		if *action.optOut {
			err = smsoptout.RecordOptOut(ctx, s.storage, from)
		} else {
			err = smsoptout.RecordOptIn(ctx, s.storage, from)
		}
		if err != nil {
			logger.ErrorContext(ctx, "sms webhook: failed to persist opt-out state", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	logger.InfoContext(ctx, "sms webhook: synced keyword to opt-out store")

	// Twilio's opt-out management sends the compliant reply; we add none.
	s.writeEmptyTwiML(w)
}

// twiMLResponse is the empty TwiML envelope that acknowledges the webhook
// without adding an app-level reply.
type twiMLResponse struct {
	XMLName xml.Name `xml:"Response"`
}

// writeEmptyTwiML acknowledges the inbound with a bare <Response/> so Twilio
// layers no reply on top of its own opt-out auto-reply.
func (s *Service) writeEmptyTwiML(w http.ResponseWriter) {
	body, err := xml.Marshal(twiMLResponse{})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
}
