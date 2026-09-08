package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignUnsubscribeToken returns a stateless, tamper-proof token authorizing the
// bearer to unsubscribe the given user from off-app email notifications. It is
// an HMAC-SHA256 of the user ID under the server's signing secret, hex-encoded.
//
// The token gates the one-click unsubscribe link embedded in off-app emails:
// the link carries (user_id, token), and the unsubscribe endpoint recomputes
// the HMAC to confirm the link was issued by us before flipping the user's
// opt-out flag. No server-side state is needed, and the user ID is not secret
// (only the ability to forge a valid token for an arbitrary user matters).
func SignUnsubscribeToken(secret []byte, userID string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("off_app_email_unsubscribe:" + userID))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyUnsubscribeToken reports whether token is a valid unsubscribe token for
// userID under secret. The comparison is constant-time.
func VerifyUnsubscribeToken(secret []byte, userID, token string) bool {
	expected := SignUnsubscribeToken(secret, userID)
	got, err := hex.DecodeString(token)
	if err != nil {
		return false
	}
	exp, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	return hmac.Equal(got, exp)
}
