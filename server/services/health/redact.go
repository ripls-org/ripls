package health

import "regexp"

// urlQueryPattern matches the query string of an http(s) URL embedded in
// free text, capturing everything before the "?".
var urlQueryPattern = regexp.MustCompile(`(https?://[^\s"'?]+)\?[^\s"']*`)

// redactURLQueries removes the query string from every URL in a dependency's
// error text. Health results are logged and served unauthenticated on
// /health, and an error from an HTTP client quotes the URL it requested;
// for a provider that takes its credential as a query parameter, that URL
// contains the credential. Checkers should not put URLs in their errors at
// all, and this is the backstop for one that does.
func redactURLQueries(text string) string {
	return urlQueryPattern.ReplaceAllString(text, "$1?[redacted]")
}
