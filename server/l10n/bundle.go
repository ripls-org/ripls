package l10n

import (
	"embed"
	"fmt"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed source/*.toml
var sourceFS embed.FS

// DefaultTag is the language tag returned by Normalize when the input
// is empty or unsupported, and the locale of the message file used as
// the bundle's default. Every other locale must define every key
// present in source/en.toml.
var DefaultTag = language.English

// supportedTags lists every locale shipped in source/. Keep in sync
// with the source/*.toml files committed to the repository.
//
// To add a new locale: drop a `source/<tag>.toml` file in, then append
// the tag here. The parity check fails CI if the new file is missing
// any key that exists in en.toml.
var supportedTags = []language.Tag{
	language.English, // en
	language.Spanish, // es
}

var (
	bundleOnce sync.Once
	bundle     *i18n.Bundle
	errBundle  error
)

// Bundle returns the process-wide go-i18n bundle loaded from the
// embedded source/*.toml files. The first call initializes the
// bundle; subsequent calls return the same instance. If loading any
// embedded message file fails, every call returns the same error
// (the bundle is unusable until the source files are fixed and the
// process restarts).
func Bundle() (*i18n.Bundle, error) {
	bundleOnce.Do(func() {
		b := i18n.NewBundle(DefaultTag)
		b.RegisterUnmarshalFunc("toml", toml.Unmarshal)

		for _, tag := range supportedTags {
			name := fmt.Sprintf("source/%s.toml", tag.String())
			data, err := sourceFS.ReadFile(name)
			if err != nil {
				errBundle = fmt.Errorf("l10n: read embedded %s: %w", name, err)
				return
			}
			// Empty TOML files (or files containing only comments) are
			// valid: ParseMessageFileBytes returns a file with no
			// messages, which AddMessages accepts as a no-op.
			if _, err := b.ParseMessageFileBytes(data, name); err != nil {
				errBundle = fmt.Errorf("l10n: parse %s: %w", name, err)
				return
			}
		}
		bundle = b
	})
	return bundle, errBundle
}

// MustBundle is like Bundle but panics on initialization failure. Use
// it in main() and other startup paths where a broken catalog should
// terminate the process loudly. Library code in request handlers
// should call Bundle and treat the error gracefully.
func MustBundle() *i18n.Bundle {
	b, err := Bundle()
	if err != nil {
		panic(err)
	}
	return b
}

// Supported reports every locale tag the bundle ships translations
// for, in stable order. Used by tests and by the parity check; not
// suitable as a runtime allow-list because the tags are
// language.Tag, not strings.
func Supported() []language.Tag {
	out := make([]language.Tag, len(supportedTags))
	copy(out, supportedTags)
	return out
}
