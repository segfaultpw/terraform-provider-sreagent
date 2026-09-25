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
