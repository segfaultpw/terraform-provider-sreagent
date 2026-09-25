package engine

import "testing"

func TestReadableNeverExposesASecret(t *testing.T) {
	got := readable([]Attr{
		{Name: "name", Kind: String},
		{Name: "api_key", Kind: String, Secret: true},
		{Name: "duration_minutes", Kind: Int, NotRead: true},
		{Name: "provider", TFName: "provider_type", Kind: String},
	})
	if _, ok := got["api_key"]; ok {
		t.Fatal("a secret is readable")
	}
	if got["api_key_set"].Kind != Bool || got["name"].Kind != String {
		t.Fatalf("got %+v", got)
	}
	if _, ok := got["duration_minutes"]; ok {
		t.Fatal("an unanswered action argument is readable")
	}
	if got["provider_type"].Name != "provider" {
		t.Fatalf("a renamed attribute must read the answer's own key, got %+v", got["provider_type"])
	}
}
