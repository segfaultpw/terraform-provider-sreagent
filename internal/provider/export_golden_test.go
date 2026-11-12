package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/zclconf/go-cty/cty"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// providerResource is one resource of this provider as the export has to see it:
// the attributes a configuration may state and the values some of them allow.
type providerResource struct {
	settable map[string]bool
	oneOf    map[string][]string
}

func providerResources(t *testing.T) map[string]providerResource {
	t.Helper()
	ctx := context.Background()
	out := map[string]providerResource{}
	oneOf := map[string]map[string][]string{}
	for _, s := range specs.All() {
		for _, a := range s.Attrs {
			if len(a.OneOf) > 0 {
				if oneOf[s.TypeName] == nil {
					oneOf[s.TypeName] = map[string][]string{}
				}
				oneOf[s.TypeName][a.Attribute()] = a.OneOf
			}
		}
	}
	for _, factory := range New("test")().(interface {
		Resources(context.Context) []func() resource.Resource
	}).Resources(ctx) {
		r := factory()
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "sreagent"}, &meta)
		var schema resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schema)
		pr := providerResource{settable: map[string]bool{}}
		for name, a := range schema.Schema.Attributes {
			// Computed only: the platform answers it, a configuration cannot state it.
			pr.settable[name] = a.IsOptional() || a.IsRequired()
		}
		pr.oneOf = oneOf[strings.TrimPrefix(meta.TypeName, "sreagent_")]
		out[meta.TypeName] = pr
	}
	return out
}

// literalStrings is the strings a literal expression holds, nothing for an
// expression that needs a function or a reference to be known.
func literalStrings(e hclsyntax.Expression) []string {
	v, diags := e.Value(nil)
	if diags.HasErrors() || v.IsNull() || !v.IsKnown() {
		return nil
	}
	switch {
	case v.Type() == cty.String:
		return []string{v.AsString()}
	case v.Type().IsTupleType() || v.Type().IsListType():
		var out []string
		for it := v.ElementIterator(); it.Next(); {
			_, el := it.Element()
			if el.Type() == cty.String {
				out = append(out, el.AsString())
			}
		}
		return out
	}
	return nil
}

func problemsInExport(t *testing.T, name string, src []byte, schemas map[string]providerResource) []string {
	t.Helper()
	file, diags := hclsyntax.ParseConfig(src, name, hcl.InitialPos)
	if diags.HasErrors() {
		return []string{diags.Error()}
	}
	var problems []string
	body := file.Body.(*hclsyntax.Body)
	declared := map[string]bool{}
	var imports []string
	for _, blk := range body.Blocks {
		switch blk.Type {
		case "resource":
			typeName, label := blk.Labels[0], blk.Labels[1]
			declared[typeName+"."+label] = true
			schema, ok := schemas[typeName]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s.%s: the provider has no resource %s", typeName, label, typeName))
				continue
			}
			for attr, a := range blk.Body.Attributes {
				settable, known := schema.settable[attr]
				switch {
				case !known:
					problems = append(problems, fmt.Sprintf("%s.%s sets %s, which the provider does not declare", typeName, label, attr))
				case !settable:
					problems = append(problems, fmt.Sprintf("%s.%s sets %s, which only the platform can set", typeName, label, attr))
				}
				if allowed := schema.oneOf[attr]; len(allowed) > 0 {
					for _, v := range literalStrings(a.Expr) {
						if !slices.Contains(allowed, v) {
							problems = append(problems, fmt.Sprintf("%s.%s sets %s to %q, which the provider refuses (one of %v)", typeName, label, attr, v, allowed))
						}
					}
				}
				if typeName == "sreagent_repository_defaults" && attr == "confirm" {
					problems = append(problems, fmt.Sprintf("%s.%s writes confirm: an exported row is already on, and a confirmation only ever belongs to a change", typeName, label))
				}
			}
			if len(blk.Body.Blocks) > 0 {
				problems = append(problems, fmt.Sprintf("%s.%s has a nested block, which no resource of this provider takes", typeName, label))
			}
		case "import":
			if attr, ok := blk.Body.Attributes["to"]; ok {
				if tr, ok := attr.Expr.(*hclsyntax.ScopeTraversalExpr); ok && len(tr.Traversal) > 1 {
					if step, ok := tr.Traversal[1].(hcl.TraverseAttr); ok {
						imports = append(imports, tr.Traversal.RootName()+"."+step.Name)
					}
				}
			}
		}
	}
	for _, to := range imports {
		if !declared[to] {
			problems = append(problems, fmt.Sprintf("an import block targets %s, which the file does not declare", to))
		}
	}
	return problems
}

// The platform's exported file is written once per release of the platform and
// adopted with imports alone. Every resource it declares has to be one this
// provider has, and every attribute one it takes: a computed-only attribute, or a
// value the provider refuses, fails the adopter's plan before it imports a row.
func TestThePlatformExportSetsOnlyWhatTheProviderTakes(t *testing.T) {
	schemas := providerResources(t)
	files, err := filepath.Glob("testdata/export_*.tf")
	if err != nil || len(files) == 0 {
		t.Fatalf("no exported file to check: %v", err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path) //nolint:gosec // a fixture of this repository
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range problemsInExport(t, path, src, schemas) {
				t.Error(p)
			}
		})
	}
}

// The check bites: each way an export can ask for more than the provider takes
// is named.
func TestTheExportCheckNamesWhatTheProviderDoesNotTake(t *testing.T) {
	schemas := providerResources(t)
	for name, c := range map[string]struct{ src, want string }{
		"an attribute the provider lacks": {`resource "sreagent_team" "t" { colour = "red" }`, "does not declare"},
		"a computed-only attribute":       {`resource "sreagent_repository_defaults" "d" { source = "own" }`, "only the platform can set"},
		"a resource the provider lacks":   {`resource "sreagent_nope" "d" {}`, "has no resource"},
		"a value outside the enum":        {`resource "sreagent_repo_setting" "r" { follows_default = ["service"] }`, "refuses"},
		"a confirmation":                  {`resource "sreagent_repository_defaults" "d" { confirm = ["ci_config"] }`, "already on"},
		"an import with no resource":      {"import {\n  to = sreagent_team.gone\n  id = \"x\"\n}", "does not declare"},
	} {
		t.Run(name, func(t *testing.T) {
			got := strings.Join(problemsInExport(t, name, []byte(c.src), schemas), "\n")
			if !strings.Contains(got, c.want) {
				t.Fatalf("want a problem containing %q, got %q", c.want, got)
			}
		})
	}
}
