// Package emailsuppression records and checks email addresses that have opted
// out of platform email but have no registered account (e.g. an email invitee
// who unsubscribed before signing up). Registered users opt out via
// User.off_app_email_opted_out; this is the non-user counterpart, consulted
// before every off-app invite email.
package emailsuppression
