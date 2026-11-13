package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// securityResourceKeys maps the type of each security resource the platform's export writes to the key the
// configuration API serves it under.
var securityResourceKeys = map[string]string{
	"sreagent_security_sla_policy":        "security_sla_policy",
	"sreagent_security_digest_settings":   "security_digest_settings",
	"sreagent_security_code_classes":      "security_code_classes",
	"sreagent_security_notification_rule": "security_notification_rules",
	"sreagent_secret_allow_rule":          "secret_allow_rules",
}

// securityNullStandIns are the two values the platform holds as null and the export states as a stand-in
// (the contract's terraform_null_as), so the seeded row holds the null the real platform holds.
var securityNullStandIns = map[string]map[string]any{
	"sreagent_security_sla_policy":      {"info_days": float64(0)},
	"sreagent_security_digest_settings": {"slack_channel": ""},
}

// securityOptionalColumns are the nullable columns a rendered block leaves out when the platform holds null.
var securityOptionalColumns = map[string][]string{
	"sreagent_security_notification_rule": {"slack_channel"},
	"sreagent_secret_allow_rule":          {"repository", "detector_type", "expires_at"},
}

type securityFileBlock struct {
	typeName, label string
	values          map[string]any
	source          []byte
}

// TestTheRenderedSecurityFileAdoptsAndChangesNothing is the proof the platform's export promises for the five security
// resources, kept where it keeps running: testdata/export_security_blocks.tf is a file the platform's own exporter
// wrote (see the header of that file), taken here verbatim as the configuration. The fake facade is seeded from the
// file itself, one row for each block under the id its import block names, with the null the real platform holds
// where the export writes a stand-in, so a change to the platform's rendering that this repository copies into
// testdata/ is proved by this test without editing it. Step one is the file with its own import blocks and must plan
// imports only; step two is the same resources with no import blocks and must plan nothing; and the fake must have
// seen no write of any kind on the five keys.
func TestTheRenderedSecurityFileAdoptsAndChangesNothing(t *testing.T) {
	src, err := os.ReadFile("testdata/export_security_blocks.tf")
	if err != nil {
		t.Fatal(err)
	}
	file, diags := hclsyntax.ParseConfig(src, "export_security_blocks.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	var resources, imports []byte
	var blocks []securityFileBlock
	ids := map[string]string{}

	for _, blk := range file.Body.(*hclsyntax.Body).Blocks {
		text := src[blk.Range().Start.Byte:blk.Range().End.Byte]
		switch blk.Type {
		case "resource":
			typeName, label := blk.Labels[0], blk.Labels[1]
			if _, ok := securityResourceKeys[typeName]; !ok {
				continue
			}
			values := map[string]any{}
			for name, attr := range blk.Body.Attributes {
				v, d := attr.Expr.Value(nil)
				if d.HasErrors() {
					t.Fatalf("%s.%s %s: %s", typeName, label, name, d.Error())
				}
				values[name] = goValue(t, v)
			}
			blocks = append(blocks, securityFileBlock{typeName: typeName, label: label, values: values, source: text})
			resources = append(resources, text...)
			resources = append(resources, '\n', '\n')
		case "import":
			tr, ok := blk.Body.Attributes["to"].Expr.(*hclsyntax.ScopeTraversalExpr)
			if !ok || len(tr.Traversal) < 2 {
				t.Fatalf("an import block with no target: %s", text)
			}
			address := tr.Traversal.RootName() + "." + tr.Traversal[1].(hcl.TraverseAttr).Name
			v, d := blk.Body.Attributes["id"].Expr.Value(nil)
			if d.HasErrors() {
				t.Fatal(d.Error())
			}
			ids[address] = v.AsString()
			imports = append(imports, text...)
			imports = append(imports, '\n', '\n')
		}
	}

	seen := map[string]int{}
	for _, b := range blocks {
		seen[b.typeName]++
	}
	for typeName := range securityResourceKeys {
		if seen[typeName] == 0 {
			t.Fatalf("the file holds no %s: the proof would not cover it", typeName)
		}
	}
	if len(ids) != len(blocks) {
		t.Fatalf("%d security blocks and %d imports", len(blocks), len(ids))
	}

	f := fakefacade.New(t, specs.All())
	classes := []any{}
	for _, b := range blocks {
		if b.typeName == "sreagent_security_code_classes" {
			classes, _ = b.values["classes"].([]any)
		}
	}
	accepted := make([]string, 0, len(classes))
	for _, c := range classes {
		accepted = append(accepted, fmt.Sprint(c))
	}
	f.SetAcceptedClasses(accepted...)

	for _, b := range blocks {
		key := securityResourceKeys[b.typeName]
		row := map[string]any{}
		for name, v := range b.values {
			row[name] = v
		}
		for name, standIn := range securityNullStandIns[b.typeName] {
			if row[name] == standIn {
				row[name] = nil
			}
		}
		for _, name := range securityOptionalColumns[b.typeName] {
			if _, stated := row[name]; !stated {
				row[name] = nil
			}
		}

		id, ok := ids[b.typeName+"."+b.label]
		if !ok {
			t.Fatalf("%s.%s has no import block", b.typeName, b.label)
		}
		if b.typeName == "sreagent_security_notification_rule" || b.typeName == "sreagent_secret_allow_rule" {
			row["id"] = id
			row["created_at"] = "2026-01-01T00:00:00.000000Z"
			f.Seed(key, row)
			continue
		}
		if b.typeName == "sreagent_security_code_classes" {
			row["available_classes"] = []any{}
		}
		f.Mutate(key, key, func(stored map[string]any) {
			for name, v := range row {
				stored[name] = v
			}
		})
	}

	noWrites := func(*terraform.State) error {
		for _, key := range securityResourceKeys {
			if n := len(f.PutBodies(key)); n != 0 {
				return fmt.Errorf("adopting the rendered file sent %d PUT bodies to %s: it must import and change nothing", n, key)
			}
			for _, method := range []string{"PUT", "POST", "DELETE"} {
				if n := f.Calls(method, key); n != 0 {
					return fmt.Errorf("adopting the rendered file sent %d %s requests to %s", n, method, key)
				}
			}
		}
		return nil
	}

	// The write count is read inside the last step: the test framework's own teardown destroys what the run created,
	// and those DELETEs are not the adoption's.
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []resource.TestStep{
			{
				Config:           providerBlock(f.URL) + string(resources) + string(imports),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{importsOnly{}}},
				Check:            noWrites,
			},
			{
				Config:           providerBlock(f.URL) + string(resources),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check:            noWrites,
			},
		},
	})
}

// goValue turns a literal HCL value into the Go value the fake facade stores, the way the facade's JSON would carry it.
func goValue(t *testing.T, v cty.Value) any {
	t.Helper()
	raw, err := ctyjson.SimpleJSONValue{Value: v}.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
