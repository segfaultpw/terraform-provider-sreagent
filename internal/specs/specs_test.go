package specs

import (
	"strings"
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

func TestAQuickRowEntryIsHeldToTheEmojiBoundsOfThePlatform(t *testing.T) {
	// SreData.Emoji.normalize/1: at most 16 code points and 64 bytes.
	fam := "\U0001F468\u200d\U0001F469\u200d\U0001F467\u200d\U0001F466" // 7 code points, 25 bytes
	cases := []struct {
		name  string
		entry string
		ok    bool
	}{
		{"one emoji", "\U0001F389", true},
		{"a family", fam, true},
		{"16 code points", strings.Repeat("a", 16), true},
		{"17 code points", strings.Repeat("a", 17), false},
		{"16 four byte code points is 64 bytes", strings.Repeat("\U0001F389", 16), true},
		{"17 four byte code points", strings.Repeat("\U0001F389", 17), false},
		{"15 three byte code points is 45 bytes", strings.Repeat("\u2764", 15), true},
		{"a Slack name", ":tada:", false},
	}
	for _, c := range cases {
		got := quickRowEntry(c.entry)
		if (got == "") != c.ok {
			t.Errorf("%s: quickRowEntry = %q, accepted = %v, want accepted = %v", c.name, got, got == "", c.ok)
		}
	}
	if n := len(strings.Repeat("\U0001F389", 16)); n != 64 {
		t.Fatalf("fixture is %d bytes, want 64", n)
	}
}

func TestTheTwoGitHubAccountResourcesSayWhichOneToUseWhen(t *testing.T) {
	for _, want := range []string{"sreagent_github_settings manages the organization's managed account", "the one connected longest", "fights sreagent_github_settings", "any other account", "terraform import sreagent_github_account.NAME"} {
		if !strings.Contains(GitHubAccount.Description, want) {
			t.Errorf("sreagent_github_account does not say %q", want)
		}
	}
	for _, want := range []string{"CHILD_ID?account=ACCOUNT", "api:config_read", "Destroying the resource leaves the row in place", "fingerprint", "adopts it"} {
		if !strings.Contains(GitHubChildAccess.Description, want) {
			t.Errorf("sreagent_github_child_access does not say %q", want)
		}
	}
	if !GitHubAccount.AdoptOnly || GitHubAccount.LeaveOnDestroy == "" || !GitHubChildAccess.AdoptOnly || GitHubChildAccess.LeaveOnDestroy == "" {
		t.Error("an account or a child grant has a door that creates or deletes it, which the platform has not")
	}
	for _, a := range append(append([]engine.Attr{}, GitHubAccount.Attrs...), GitHubChildAccess.Attrs...) {
		if a.Name == "context_repo" {
			t.Error("context_repo is an action argument of the account row, not an attribute: sreagent_github_settings sets it")
		}
	}
}

// No description carries a dash used as punctuation (an em or en dash), which the
// platform's own prose check refuses in anything committed.
func TestNoSpecDescriptionUsesAnEmOrEnDash(t *testing.T) {
	for _, s := range All() {
		texts := []string{s.Description, s.ReadDescription}
		for _, list := range [][]engine.Attr{s.Attrs, s.ListAttrs, s.ExtraAttrs, s.QueryAttrs} {
			for _, a := range list {
				texts = append(texts, a.Description, a.ReadDescription)
			}
		}
		for _, text := range texts {
			if strings.ContainsAny(text, "\u2014\u2013") {
				t.Errorf("%s: a description uses an em or en dash: %.60s", s.Key, text)
			}
		}
	}
}
