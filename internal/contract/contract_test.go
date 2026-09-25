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

// specProblems is every way one spec disagrees with the contract document.
func specProblems(d *Doc, s engine.Spec) []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }
	res, ok := d.Resources[s.Key]
	if !ok {
		return append(p, fmt.Sprintf("%s is not in the API contract", s.Key))
	}
	if want := res.TerraformType != ""; s.Lifecycle != want {
		return append(p, fmt.Sprintf("Lifecycle %v, but the contract's terraform_type is %q", s.Lifecycle, res.TerraformType))
	}
	if s.Lifecycle && s.TypeName != res.TerraformType {
		return append(p, fmt.Sprintf("TypeName %q, contract terraform_type %q", s.TypeName, res.TerraformType))
	}
	if (s.Shape == engine.Singleton) != res.Singleton {
		return append(p, "singleton mismatch")
	}
	if s.Lifecycle && s.Upsert != res.Upsert {
		return append(p, fmt.Sprintf("Upsert %v, contract upsert %v", s.Upsert, res.Upsert))
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
			add("field %s is missing from the spec", f)
			continue
		}
		if a.Computed || a.Secret {
			if s.Lifecycle && !isReadOnlyByDesign(s.Key, f) {
				add("field %s is writable in the contract but %s in the spec", f, map[bool]string{true: "computed", false: "secret"}[a.Computed])
			}
		}
		if got, want := a.Kind, kindFor(row[f]); got != want {
			add("field %s kind %d, contract %d", f, got, want)
		}
		if s.Lifecycle && s.Shape != engine.Singleton {
			if a.Required != slices.Contains(post.Required, f) {
				add("field %s Required %v, contract required %v", f, a.Required, post.Required)
			}
			createOnly := slices.Contains(res.CreateFields, f) && !slices.Contains(res.UpdateFields, f)
			if len(res.UpdateFields) == 0 {
				createOnly = slices.Contains(res.CreateFields, f)
			}
			if a.CreateOnly != createOnly {
				add("field %s CreateOnly %v, contract %v", f, a.CreateOnly, createOnly)
			}
			updateOnly := !slices.Contains(res.CreateFields, f) && slices.Contains(res.UpdateFields, f)
			if a.UpdateOnly != updateOnly {
				add("field %s UpdateOnly %v, contract %v", f, a.UpdateOnly, updateOnly)
			}
			_, nullable := typeOf(put.Properties[f])
			if a.Clearable != nullable {
				add("field %s Clearable %v, update body nullable %v", f, a.Clearable, nullable)
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
				add("field %s OneOf %v, contract enum %v", f, a.OneOf, want)
			}
		} else if len(a.OneOf) > 0 {
			add("field %s has OneOf but the contract declares no enum", f)
		}
	}
	for _, f := range res.SecretFields {
		if a, ok := byName[f]; !ok || !a.Secret {
			add("secret %s must be a Secret attribute", f)
		}
	}
	for _, a := range s.Attrs {
		switch {
		case a.NotRead:
			if !slices.Contains(res.ActionArgs, a.Name) {
				add("NotRead %s is not an action argument", a.Name)
			}
		case a.Computed:
			if _, ok := res.ComputedFields[a.Name]; !ok && !slices.Contains(res.Fields, a.Name) {
				add("computed %s is not answered by the API", a.Name)
			}
		case a.Secret:
		default:
			if !slices.Contains(res.Fields, a.Name) {
				add("attribute %s is not a field of %s", a.Name, s.Key)
			}
		}
	}
	checkSecrets(d, s, res, add)
	checkAddressing(s, res, add)
	return p
}

func TestEverySpecMatchesTheContract(t *testing.T) {
	d := doc(t)
	for _, s := range specs.All() {
		t.Run(s.Key, func(t *testing.T) {
			for _, problem := range specProblems(d, s) {
				t.Error(problem)
			}
		})
	}
}

// checkSecrets holds each Secret attribute to secret_fields, and a secret's
// CreateOnly and Required to the create and update fields and the POST body.
func checkSecrets(d *Doc, s engine.Spec, res Resource, add func(string, ...any)) {
	post := d.Paths["/"+s.Key]["post"].RequestBody.Content["application/json"].Schema
	for _, a := range s.Attrs {
		if !a.Secret {
			continue
		}
		if !slices.Contains(res.SecretFields, a.Name) {
			add("Secret %s is not in the contract's secret_fields", a.Name)
		}
		if !s.Lifecycle || s.Shape == engine.Singleton {
			continue
		}
		createOnly := slices.Contains(res.CreateFields, a.Name) && !slices.Contains(res.UpdateFields, a.Name)
		if a.CreateOnly != createOnly {
			add("secret %s CreateOnly %v, contract %v", a.Name, a.CreateOnly, createOnly)
		}
		if a.Required != slices.Contains(post.Required, a.Name) {
			add("secret %s Required %v, contract required %v", a.Name, a.Required, post.Required)
		}
	}
}

// checkAddressing holds a spec's Shape and IDAttr to how the contract
// addresses a row: id_args and natural_key.
func checkAddressing(s engine.Spec, res Resource, add func(string, ...any)) {
	var want []string
	switch s.Shape {
	case engine.Singleton:
		want = []string{}
	case engine.Generated:
		want = []string{"id"}
	case engine.NaturalKey:
		want = []string{s.IDAttr}
		if !slices.Equal(res.NaturalKey, want) {
			add("natural key %v, contract natural_key %v", want, res.NaturalKey)
		}
	case engine.ServiceBinding:
		want = []string{"service", "environment"}
	}
	got := res.IDArgs
	if got == nil {
		got = []string{}
	}
	if !slices.Equal(got, want) {
		add("Shape %d addresses a row by %v, contract id_args %v", s.Shape, want, got)
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

// The contract test must fail when the document drifts from a spec: each
// mutation below is a change the server could ship.
func TestTheContractTestCatchesDrift(t *testing.T) {
	for _, c := range []struct {
		name, key string
		mutate    func(d *Doc)
	}{
		{"a secret leaves secret_fields", "outbound_configs", func(d *Doc) {
			r := d.Resources["outbound_configs"]
			r.SecretFields = slices.DeleteFunc(slices.Clone(r.SecretFields), func(f string) bool { return f == "routing_key" })
			d.Resources["outbound_configs"] = r
		}},
		{"a secret leaves update_fields", "connectors", func(d *Doc) {
			r := d.Resources["connectors"]
			r.UpdateFields = slices.DeleteFunc(slices.Clone(r.UpdateFields), func(f string) bool { return f == "config" })
			d.Resources["connectors"] = r
		}},
		{"a secret stops being required", "connectors", func(d *Doc) {
			op := d.Paths["/connectors"]["post"]
			body := op.RequestBody.Content["application/json"]
			body.Schema.Required = slices.DeleteFunc(slices.Clone(body.Schema.Required), func(f string) bool { return f == "config" })
			op.RequestBody.Content["application/json"] = body
			d.Paths["/connectors"]["post"] = op
		}},
		{"a natural key becomes a generated id", "ticket_integrations", func(d *Doc) {
			r := d.Resources["ticket_integrations"]
			r.IDArgs = []string{"id"}
			d.Resources["ticket_integrations"] = r
		}},
		{"the natural key changes", "ticket_import_rules", func(d *Doc) {
			r := d.Resources["ticket_import_rules"]
			r.NaturalKey = []string{"name"}
			d.Resources["ticket_import_rules"] = r
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := doc(t)
			var spec engine.Spec
			for _, s := range specs.All() {
				if s.Key == c.key {
					spec = s
				}
			}
			if problems := specProblems(d, spec); len(problems) != 0 {
				t.Fatalf("the untouched document already disagrees: %v", problems)
			}
			c.mutate(d)
			if len(specProblems(d, spec)) == 0 {
				t.Fatal("the contract test missed this drift")
			}
		})
	}
}
