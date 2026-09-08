// Package smsoptout records and checks the global text-message opt-out state
// for a phone number, keyed by its normalized E.164 form. A recipient texting
// STOP is suppressed everywhere (even across different hosts) until they text
// START; the state is consulted before every platform text send. This is the
// SMS counterpart to server/emailsuppression.
package smsoptout
