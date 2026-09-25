package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
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

func TestEveryLongStringOfAJSONSecretIsRedactable(t *testing.T) {
	got := stringLeaves(map[string]any{"host": "10.0.0.1", "auth": map[string]any{"password": "hunter22", "keys": []any{"k-123456"}}, "port": 22, "u": "sre"})
	for _, want := range []string{"10.0.0.1", "hunter22", "k-123456"} {
		if !slices.Contains(got, want) {
			t.Errorf("%s is missing from %v", want, got)
		}
	}
	if slices.Contains(got, "sre") {
		t.Errorf("a string too short to redact safely was kept: %v", got)
	}
}

func TestADroppedKeyOfAMergedObjectIsCleared(t *testing.T) {
	prior := jsontypes.NewNormalizedValue(`{"investigation":"a","suggestion":"b"}`)
	got := clearRemovedKeys(map[string]any{"investigation": "a"}, prior).(map[string]any)
	if got["suggestion"] != "" || got["investigation"] != "a" {
		t.Fatalf("got %v", got)
	}
}
