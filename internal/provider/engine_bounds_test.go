package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

func ptr(n int64) *int64 { return &n }

var scratchSLA = engine.Spec{
	Key: "scratch_sla", TypeName: "scratch_sla", ListName: "scratch_sla", Shape: engine.Singleton, Lifecycle: true,
	Description: "A scratch singleton whose null is a real setting.",
	Attrs: []engine.Attr{
		{Name: "days", Kind: engine.Int, NullAs: "0", Min: ptr(1), Max: ptr(365), Description: "1 to 365, or 0 for none."},
		{Name: "other", Kind: engine.Int, Description: "Another setting."},
	},
}

func TestANullAsValueRoundTripsAndPlansNothing(t *testing.T) {
	f, factories := scratch(t, scratchSLA)
	cfg := func(body string) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_scratch_sla\" \"s\" {\n%s\n}\n", body)
	}
	empty := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:           cfg("days = 0\nother = 5"),
				ConfigPlanChecks: empty,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("sreagent_scratch_sla.s", "days", "0"),
					func(*terraform.State) error {
						if row := f.Row("scratch_sla", "scratch_sla"); row["days"] != nil {
							return fmt.Errorf("days = 0 must be stored as null, got %v", row["days"])
						}
						if body := f.LastBody("PUT", "scratch_sla"); body["days"] != nil || !hasKey(body, "days") {
							return fmt.Errorf("days = 0 must be sent as null, got %v", body)
						}
						return nil
					},
				),
			},
			{Config: cfg("days = 0\nother = 5"), PlanOnly: true},
			{Config: cfg("days = 7\nother = 5"), Check: resource.TestCheckResourceAttr("sreagent_scratch_sla.s", "days", "7")},
			{
				Config: cfg("days = 0\nother = 5"),
				Check: func(*terraform.State) error {
					if body := f.LastBody("PUT", "scratch_sla"); !hasKey(body, "days") || body["days"] != nil {
						return fmt.Errorf("going back to 0 sends null, got %v", body)
					}
					return nil
				},
			},
			{
				// Leaving the attribute out leaves it as it is: nothing is planned and nothing is sent for it.
				Config:           cfg("other = 6"),
				ConfigPlanChecks: empty,
				Check: func(*terraform.State) error {
					if body := f.LastBody("PUT", "scratch_sla"); hasKey(body, "days") {
						return fmt.Errorf("an attribute left out must not be sent, got %v", body)
					}
					return nil
				},
			},
			{Config: cfg("days = 500"), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)days.*between 1 and 365`)},
			{Config: cfg("days = -3"), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)days.*between 1 and 365`)},
			{Config: cfg("days = 30\nother = 6"), Check: resource.TestCheckResourceAttr("sreagent_scratch_sla.s", "days", "30")},
		},
	})
}

var scratchRule = engine.Spec{
	Key: "scratch_rules", TypeName: "scratch_rule", ListName: "scratch_rules", Shape: engine.Generated, Lifecycle: true,
	Description: "A scratch collection with published defaults and bounds.",
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, MinLength: 1, MaxLength: 5, Description: "The name."},
		{Name: "enabled", Kind: engine.Bool, Default: "true", Description: "On."},
		{Name: "min_severity", Kind: engine.String, Default: `"high"`, OneOf: []string{"low", "high"}, Description: "Severity."},
		{Name: "events", Kind: engine.StringList, Default: `["new_finding"]`, MinItems: 1, OneOf: []string{"new_finding", "sla_breached"}, Description: "Events."},
	},
}

func TestADefaultIsSentOnCreateAndRestoredWhenAStatedValueIsRemoved(t *testing.T) {
	f, factories := scratch(t, scratchRule)
	cfg := func(body string) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_scratch_rule\" \"r\" {\n  name = \"a\"\n%s\n}\n", body)
	}
	empty := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:           cfg(""),
				ConfigPlanChecks: empty,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("sreagent_scratch_rule.r", "enabled", "true"),
					resource.TestCheckResourceAttr("sreagent_scratch_rule.r", "min_severity", "high"),
					resource.TestCheckResourceAttr("sreagent_scratch_rule.r", "events.#", "1"),
					func(*terraform.State) error {
						body := f.LastBody("POST", "scratch_rules")
						if body["enabled"] != true || body["min_severity"] != "high" {
							return fmt.Errorf("the defaults must be sent on create, got %v", body)
						}
						return nil
					},
				),
			},
			{
				Config: cfg("enabled = false\nmin_severity = \"low\"\nevents = [\"new_finding\", \"sla_breached\"]"),
				Check:  resource.TestCheckResourceAttr("sreagent_scratch_rule.r", "enabled", "false"),
			},
			{
				// Removing the stated values plans the defaults back, and sends them.
				Config:           cfg(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("sreagent_scratch_rule.r", plancheck.ResourceActionUpdate)}},
				Check: func(*terraform.State) error {
					body := f.LastBody("PUT", "scratch_rules")
					if body["enabled"] != true || body["min_severity"] != "high" {
						return fmt.Errorf("a removed value must send the default, got %v", body)
					}
					return nil
				},
			},
			{Config: cfg(""), PlanOnly: true},
			{Config: cfg("events = []"), PlanOnly: true, ExpectError: regexp.MustCompile(`(?is)events.*at least 1`)},
			{Config: cfg("events = [\"bogus\"]"), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)events.*bogus`)},
			{Config: cfg("min_severity = \"medium\""), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)min_severity.*medium`)},
			{
				Config:      providerBlock(f.URL) + "resource \"sreagent_scratch_rule\" \"r\" {\n  name = \"123456\"\n}\n",
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)name.*1 to 5 characters`),
			},
			{Config: cfg("min_severity = \"low\""), Check: resource.TestCheckResourceAttr("sreagent_scratch_rule.r", "min_severity", "low")},
		},
	})
}

func hasKey(m map[string]any, k string) bool { _, ok := m[k]; return ok }
