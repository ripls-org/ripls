package l10n

import (
	"strings"
	"testing"
)

// englishOnlyParams are template params whose value is composed in English and
// therefore cannot appear in a translated catalog.
//
// GearWithArticle prefixes the English indefinite article ("a Tent" / "an
// Umbrella"). Interpolating it into a Spanish sentence produced "Sam está
// regalando a Power Drill" — an English article spliced into Spanish, which
// reads as a typo to a Spanish speaker and is invisible to an English reviewer.
// Non-English copy interpolates the bare name instead and supplies whatever
// article its own grammar wants.
var englishOnlyParams = []string{"GearWithArticle"}

// TestNonEnglishCatalogsAvoidEnglishOnlyParams walks every locale but English
// and fails on any message that interpolates a param whose value is English.
func TestNonEnglishCatalogsAvoidEnglishOnlyParams(t *testing.T) {
	for _, tag := range supportedTags {
		if tag == DefaultTag {
			continue
		}
		name := "source/" + tag.String() + ".toml"
		data, err := sourceFS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var key string
		for i, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "[") {
				key = strings.Trim(line, "[]")
				continue
			}
			for _, param := range englishOnlyParams {
				if strings.Contains(line, "{{."+param+"}}") {
					t.Errorf("%s:%d (%s) interpolates {{.%s}}, whose value is composed "+
						"in English; use the bare name and supply the article in this "+
						"locale's own grammar", name, i+1, key, param)
				}
			}
		}
	}
}
