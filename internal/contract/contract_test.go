package contract

import (
	"fmt"
	"slices"
	"testing"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

func doc(t *testing.T) *Doc {
	t.Helper()
	d, err := Load("openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func typeOf(schema map[string]any) (string, bool) {
	switch t := schema["type"].(type) {
	case string:
		return t, false
	case []any:
		nullable := slices.Contains(t, any("null"))
		for _, x := range t {
			if x != "null" {
				return fmt.Sprint(x), nullable
			}
		}
	}
	return "", false
}

func kindFor(schema map[string]any) engine.Kind {
	t, _ := typeOf(schema)
	switch t {
	case "string":
		return engine.String
	case "integer":
		return engine.Int
	case "number":
		return engine.Float
	case "boolean":
		return engine.Bool
	case "array":
		if items, ok := schema["items"].(map[string]any); ok && items["type"] == "string" {
			return engine.StringList
		}
	}
	return engine.JSON
}

func TestEverySpecMatchesTheContract(t *testing.T) {
	d := doc(t)
	for _, s := range specs.All() {
		t.Run(s.Key, func(t *testing.T) {
			res, ok := d.Resources[s.Key]
			if !ok {
				t.Fatalf("%s is not in the API contract", s.Key)
			}
			if want := res.TerraformType != ""; s.Lifecycle != want {
				t.Fatalf("Lifecycle %v, but the contract's terraform_type is %q", s.Lifecycle, res.TerraformType)
			}
			if s.Lifecycle && s.TypeName != res.TerraformType {
				t.Fatalf("TypeName %q, contract terraform_type %q", s.TypeName, res.TerraformType)
			}
			if (s.Shape == engine.Singleton) != res.Singleton {
				t.Fatalf("singleton mismatch")
			}
			if s.Lifecycle && s.Upsert != res.Upsert {
				t.Fatalf("Upsert %v, contract upsert %v", s.Upsert, res.Upsert)
			}
			row := d.Components.Schemas[s.Key+"_row"].Properties
			post := d.Paths["/"+s.Key]["post"].RequestBody.Content["application/json"].Schema
			// Clearable is read from the update body: a row schema allows null wherever
			// a read can answer null, but only a field the PUT body declares nullable
			// accepts the null an unset Clearable attribute sends.
			put := d.Paths["/"+s.Key+"/{id}"]["put"].RequestBody.Content["application/json"].Schema
			// A singleton has no POST; its enums live on its PUT body.
			enums := post
			if s.Shape == engine.Singleton {
				enums = d.Paths["/"+s.Key]["put"].RequestBody.Content["application/json"].Schema
			}
			byName := map[string]engine.Attr{}
			for _, a := range s.Attrs {
				byName[a.Name] = a
			}
			for _, f := range res.Fields {
				a, ok := byName[f]
				if !ok {
					t.Errorf("field %s is missing from the spec", f)
					continue
				}
				if a.Computed || a.Secret {
					if s.Lifecycle && !isReadOnlyByDesign(s.Key, f) {
						t.Errorf("field %s is writable in the contract but %s in the spec", f, map[bool]string{true: "computed", false: "secret"}[a.Computed])
					}
				}
				if got, want := a.Kind, kindFor(row[f]); got != want {
					t.Errorf("field %s kind %d, contract %d", f, got, want)
				}
				if s.Lifecycle && s.Shape != engine.Singleton {
					if a.Required != slices.Contains(post.Required, f) {
						t.Errorf("field %s Required %v, contract required %v", f, a.Required, post.Required)
					}
					createOnly := slices.Contains(res.CreateFields, f) && !slices.Contains(res.UpdateFields, f)
					if len(res.UpdateFields) == 0 {
						createOnly = slices.Contains(res.CreateFields, f)
					}
					if a.CreateOnly != createOnly {
						t.Errorf("field %s CreateOnly %v, contract %v", f, a.CreateOnly, createOnly)
					}
					updateOnly := !slices.Contains(res.CreateFields, f) && slices.Contains(res.UpdateFields, f)
					if a.UpdateOnly != updateOnly {
						t.Errorf("field %s UpdateOnly %v, contract %v", f, a.UpdateOnly, updateOnly)
					}
					_, nullable := typeOf(put.Properties[f])
					if a.Clearable != nullable {
						t.Errorf("field %s Clearable %v, update body nullable %v", f, a.Clearable, nullable)
					}
				}
				if !s.Lifecycle {
					continue
				}
				if enum, ok := enums.Properties[f]["enum"].([]any); ok {
					want := make([]string, 0, len(enum))
					for _, e := range enum {
						want = append(want, fmt.Sprint(e))
					}
					if !slices.Equal(a.OneOf, want) {
						t.Errorf("field %s OneOf %v, contract enum %v", f, a.OneOf, want)
					}
				} else if len(a.OneOf) > 0 {
					t.Errorf("field %s has OneOf but the contract declares no enum", f)
				}
			}
			for _, f := range res.SecretFields {
				if a, ok := byName[f]; !ok || !a.Secret {
					t.Errorf("secret %s must be a Secret attribute", f)
				}
			}
			for _, a := range s.Attrs {
				switch {
				case a.NotRead:
					if !slices.Contains(res.ActionArgs, a.Name) {
						t.Errorf("NotRead %s is not an action argument", a.Name)
					}
				case a.Computed:
					if _, ok := res.ComputedFields[a.Name]; !ok && !slices.Contains(res.Fields, a.Name) {
						t.Errorf("computed %s is not answered by the API", a.Name)
					}
				case a.Secret:
				default:
					if !slices.Contains(res.Fields, a.Name) {
						t.Errorf("attribute %s is not a field of %s", a.Name, s.Key)
					}
				}
			}
		})
	}
}

// Fields the API accepts that the provider deliberately exposes read-only:
// the platform refuses these two from any connection without a user
// identity, and a Terraform key is an API key.
func isReadOnlyByDesign(key, field string) bool {
	return key == "organization_settings" && (field == "social_joins_enabled" || field == "restrict_domain_signups")
}

func TestEveryContractResourceHasASpec(t *testing.T) {
	d := doc(t)
	have := map[string]bool{}
	for _, s := range specs.All() {
		have[s.Key] = true
	}
	for key := range d.Resources {
		if !have[key] {
			t.Errorf("the API serves %s but the provider has no spec for it", key)
		}
	}
}
