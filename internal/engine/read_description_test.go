package engine

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// A resource that states a null as a value, or speaks of writing, describes a read in the data
// source's own words, so the data source's documentation stays true of what it reads.
func TestTheDataSourceUsesTheReadDescription(t *testing.T) {
	spec := Spec{Key: "k", TypeName: "k", ListName: "k", Shape: Singleton, Description: "Manage it.", ReadDescription: "Read it.",
		Attrs: []Attr{{Name: "days", Kind: Int, NullAs: "0", Description: "0 for none.", ReadDescription: "null for none."}, {Name: "plain", Kind: String, Description: "Plain."}}}
	got := readable(spec.Attrs)
	if got["days"].Description != "null for none." || got["plain"].Description != "Plain." {
		t.Errorf("the data source describes a read: %+v", got)
	}
	if d := (&facadeDataSource{spec: spec}).description(); d != "Read it." {
		t.Errorf("the data source says %q", d)
	}
	if d := (&facadeDataSource{spec: Spec{Description: "Same."}}).description(); d != "Same." {
		t.Errorf("without a read description the data source says the resource's: %q", d)
	}
	var resp resource.SchemaResponse
	NewResource(spec)().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Schema.Description != "Manage it." || resp.Schema.Attributes["days"].(rschema.Int64Attribute).Description != "0 for none." {
		t.Errorf("the resource keeps its own words: %+v", resp.Schema)
	}
}
