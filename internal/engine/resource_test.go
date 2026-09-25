package engine

import (
	"strings"
	"testing"
)

func TestDeleteWarningForDiscoveredRows(t *testing.T) {
	spec := Spec{TypeName: "image_target", DiscoveredAttr: "discovered"}
	if w := discoveredWarning(spec, true); !strings.Contains(w, "the next sweep files it again") {
		t.Fatalf("a discovered row must warn, got %q", w)
	}
	if w := discoveredWarning(spec, false); w != "" {
		t.Fatalf("a row created here must not warn, got %q", w)
	}
	if w := discoveredWarning(Spec{TypeName: "team"}, true); w != "" {
		t.Fatalf("a spec without discovered rows must not warn, got %q", w)
	}
}
