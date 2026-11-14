package specs

import (
	"testing"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// Terraform refuses a schema whose root attributes use these names.
var reserved = map[string]bool{"provider": true, "count": true, "for_each": true, "depends_on": true, "lifecycle": true, "provisioner": true, "connection": true}

func TestNoAttributeUsesAReservedName(t *testing.T) {
	for _, s := range All() {
		for _, list := range [][]engine.Attr{s.Attrs, s.ListAttrs} {
			for _, a := range list {
				if reserved[a.Attribute()] {
					t.Errorf("%s.%s is reserved in Terraform; give it a TFName", s.Key, a.Name)
				}
			}
		}
	}
}

func TestTypeNamesAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, s := range All() {
		for _, name := range []string{s.TypeName, s.ListName} {
			if prev, dup := seen[name]; dup && prev != s.Key {
				t.Errorf("%s and %s both claim sreagent_%s", prev, s.Key, name)
			}
			seen[name] = s.Key
		}
	}
}

func TestEmojiFormIgnoresThePresentationSelectorsAndNothingElse(t *testing.T) {
	cases := map[string]string{
		"❤\ufe0f":                          "❤",
		"❤":                                "❤",
		"❤\ufe0e":                          "❤",
		"\U0001F3F3\ufe0f\u200d\U0001F308": "\U0001F3F3\u200d\U0001F308",
		"\U0001F44D\U0001F3FD":             "\U0001F44D\U0001F3FD",
		"\U0001F468\u200d\U0001F469\u200d\U0001F467": "\U0001F468\u200d\U0001F469\u200d\U0001F467",
	}
	for in, want := range cases {
		if got := emojiForm(in); got != want {
			t.Errorf("emojiForm(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestASlackNameIsRefusedAndAnEmojiIsNot(t *testing.T) {
	for _, name := range []string{":tada:", ":+1:", ":white_check_mark:"} {
		if slackName(name) == "" {
			t.Errorf("%q was accepted", name)
		}
	}
	for _, ok := range []string{"\U0001F389", "❤", ":", "a:b", ":open", "close:"} {
		if got := slackName(ok); got != "" {
			t.Errorf("%q was refused: %s", ok, got)
		}
	}
}
