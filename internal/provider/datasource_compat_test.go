package provider

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// The six data sources of 0.2.0 that read the security and code detection settings. 0.3.0 adds
// resources over the same specs, and none of them may move a data source: a configuration that
// reads one keeps planning the way it did.
var securityDataSources = []string{
	"sreagent_code_detection_settings",
	"sreagent_secret_allow_rules",
	"sreagent_security_code_classes",
	"sreagent_security_digest_settings",
	"sreagent_security_notification_rules",
	"sreagent_security_sla_policy",
}

type attrShape struct {
	Type      json.RawMessage `json:"type,omitempty"`
	Computed  bool            `json:"computed"`
	Optional  bool            `json:"optional"`
	Required  bool            `json:"required"`
	Sensitive bool            `json:"sensitive"`
	Nested    *nestedShape    `json:"nested,omitempty"`
}

type nestedShape struct {
	Nesting    string               `json:"nesting"`
	Attributes map[string]attrShape `json:"attributes"`
}

func shapeOf(a *tfprotov6.SchemaAttribute) (attrShape, error) {
	out := attrShape{Computed: a.Computed, Optional: a.Optional, Required: a.Required, Sensitive: a.Sensitive}
	if a.NestedType != nil {
		nesting := map[tfprotov6.SchemaObjectNestingMode]string{
			tfprotov6.SchemaObjectNestingModeSingle: "single",
			tfprotov6.SchemaObjectNestingModeList:   "list",
			tfprotov6.SchemaObjectNestingModeSet:    "set",
			tfprotov6.SchemaObjectNestingModeMap:    "map",
		}[a.NestedType.Nesting]
		nested := &nestedShape{Nesting: nesting, Attributes: map[string]attrShape{}}
		for _, inner := range a.NestedType.Attributes {
			s, err := shapeOf(inner)
			if err != nil {
				return out, err
			}
			nested.Attributes[inner.Name] = s
		}
		out.Nested = nested
		return out, nil
	}
	raw, err := json.Marshal(typeJSON(a.Type))
	if err != nil {
		return out, err
	}
	out.Type = raw
	return out, nil
}

// typeJSON writes a type the way terraform providers schema -json does ("string", ["list","string"]).
func typeJSON(t tftypes.Type) any {
	switch {
	case t.Is(tftypes.String):
		return "string"
	case t.Is(tftypes.Number):
		return "number"
	case t.Is(tftypes.Bool):
		return "bool"
	case t.Is(tftypes.DynamicPseudoType):
		return "dynamic"
	}
	switch v := t.(type) {
	case tftypes.List:
		return []any{"list", typeJSON(v.ElementType)}
	case tftypes.Set:
		return []any{"set", typeJSON(v.ElementType)}
	case tftypes.Map:
		return []any{"map", typeJSON(v.ElementType)}
	case tftypes.Object:
		attrs := map[string]any{}
		for name, inner := range v.AttributeTypes {
			attrs[name] = typeJSON(inner)
		}
		return []any{"object", attrs}
	}
	return "unknown " + t.String()
}

// canonical re-encodes through a generic value so key order and whitespace never matter.
func canonical(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	out, err := json.MarshalIndent(generic, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func currentDataSourceShapes(t *testing.T) map[string]map[string]attrShape {
	t.Helper()
	server := providerserver.NewProtocol6(New("test")())()
	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]attrShape{}
	for _, name := range securityDataSources {
		schema, ok := resp.DataSourceSchemas[name]
		if !ok {
			t.Fatalf("the provider no longer has data.%s", name)
		}
		if len(schema.Block.BlockTypes) > 0 {
			t.Fatalf("data.%s has nested blocks, which 0.2.0 did not", name)
		}
		attrs := map[string]attrShape{}
		for _, a := range schema.Block.Attributes {
			s, err := shapeOf(a)
			if err != nil {
				t.Fatal(err)
			}
			attrs[a.Name] = s
		}
		out[name] = attrs
	}
	return out
}

// schemaMoves names what differs between two data source schemas, nothing when they are equal.
func schemaMoves(t *testing.T, want, got map[string]attrShape) []string {
	t.Helper()
	var moved []string
	for attr := range want {
		if _, ok := got[attr]; !ok {
			moved = append(moved, "lost "+attr)
		} else if canonical(t, want[attr]) != canonical(t, got[attr]) {
			moved = append(moved, "changed "+attr)
		}
	}
	for attr := range got {
		if _, ok := want[attr]; !ok {
			moved = append(moved, "added "+attr)
		}
	}
	slices.Sort(moved)
	return moved
}

// 0.3.0 manages the same settings as resources, from the same specs. The data sources are the
// part of 0.2.0 a configuration may already read, so their attribute names, types and flags are
// pinned to what 0.2.0 declared (testdata/datasources_v0_2_0.json, read from the 0.2.0 build with
// terraform providers schema -json). If this fails, fix the spec: never the file.
func TestTheSecurityDataSourcesKeepTheir020Schema(t *testing.T) {
	raw, err := os.ReadFile("testdata/datasources_v0_2_0.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]map[string]attrShape
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(securityDataSources) {
		t.Fatalf("the fixture holds %d data sources, want the six of 0.2.0", len(want))
	}
	got := currentDataSourceShapes(t)
	for _, name := range securityDataSources {
		t.Run(name, func(t *testing.T) {
			if moved := schemaMoves(t, want[name], got[name]); len(moved) > 0 {
				t.Fatalf("data.%s no longer has its 0.2.0 schema: %s", name, strings.Join(moved, ", "))
			}
		})
	}
}

// The check bites: a data source that gained an attribute, lost one or changed a flag or a type is named.
func TestTheDataSourceSchemaCheckNamesWhatMoved(t *testing.T) {
	base := currentDataSourceShapes(t)["sreagent_security_sla_policy"]
	for name, c := range map[string]struct {
		mutate func(map[string]attrShape)
		want   string
	}{
		"a lost attribute":    {func(m map[string]attrShape) { delete(m, "info_days") }, "lost info_days"},
		"an added attribute":  {func(m map[string]attrShape) { m["extra"] = attrShape{Computed: true} }, "added extra"},
		"a flag that flipped": {func(m map[string]attrShape) { a := m["info_days"]; a.Optional = true; m["info_days"] = a }, "changed info_days"},
		"a type that changed": {func(m map[string]attrShape) {
			a := m["info_days"]
			a.Type = json.RawMessage(`"string"`)
			m["info_days"] = a
		}, "changed info_days"},
	} {
		t.Run(name, func(t *testing.T) {
			copied := map[string]attrShape{}
			for k, v := range base {
				copied[k] = v
			}
			c.mutate(copied)
			if got := strings.Join(schemaMoves(t, base, copied), ","); !strings.Contains(got, c.want) {
				t.Fatalf("moves = %q, want it to name %q", got, c.want)
			}
		})
	}
	if moved := schemaMoves(t, base, base); len(moved) != 0 {
		t.Fatalf("an unchanged schema moved: %v", moved)
	}
}

// The two nulls that are real values on the resources (no SLA, no digest channel) are still null
// on the data sources, which is what 0.2.0 users read. A resource and a data source over the same
// setting in one configuration agree on the platform's answer and not on the sentinel.
func TestTheDataSourcesStillReadNullWhereTheResourcesStateZeroAndEmpty(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	const slaAddr, digestAddr = "data.sreagent_security_sla_policy.p", "data.sreagent_security_digest_settings.d"
	cfg := providerBlock(f.URL) + `
resource "sreagent_security_sla_policy" "p" {
  critical_days = 7
  high_days     = 30
  medium_days   = 90
  low_days      = 180
  info_days     = 0
}

resource "sreagent_security_digest_settings" "d" {
  slack_channel = ""
}

data "sreagent_security_sla_policy" "p" {
  depends_on = [sreagent_security_sla_policy.p]
}

data "sreagent_security_digest_settings" "d" {
  depends_on = [sreagent_security_digest_settings.d]
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:           cfg,
			ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("sreagent_security_sla_policy.p", "info_days", "0"),
				resource.TestCheckNoResourceAttr(slaAddr, "info_days"),
				resource.TestCheckResourceAttr(slaAddr, "critical_days", "7"),
				resource.TestCheckResourceAttr("sreagent_security_digest_settings.d", "slack_channel", ""),
				resource.TestCheckNoResourceAttr(digestAddr, "slack_channel"),
				resource.TestCheckResourceAttr(digestAddr, "cadence", "weekly"),
			),
		}},
	})
}
