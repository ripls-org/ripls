package email

import (
	_ "embed"
	"fmt"
	"html/template"
)

// sharedPartialsHTML holds the fragments every HTML email draws on — the
// document head and the logo lockups. Kept in one file so a rendering fix
// lands once rather than once per template: the mis-centered logo of #2927 had
// to be fixed in six places because the header was copy-pasted.
//
//go:embed partials.html
var sharedPartialsHTML string

// parseEmailHTML parses one HTML email body with the shared partials available
// to it, under the given template name.
//
// The partials are parsed first: a template body of only whitespace and
// comments does not replace an existing body, so the subsequent parse of the
// email itself supplies the body while the named defines remain callable.
func parseEmailHTML(name, body string) (*template.Template, error) {
	tmpl, err := template.New(name).Parse(sharedPartialsHTML)
	if err != nil {
		return nil, fmt.Errorf("parse shared email partials: %w", err)
	}
	if _, err := tmpl.Parse(body); err != nil {
		return nil, err
	}
	return tmpl, nil
}
