package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// A configuration written for 0.7.0 does not mention the fields 0.8.0 declares on
// existing resources. The platform may already hold values for them (set in the app),
// and neither a plan nor an apply may touch them: the plan is empty, and an apply
// that changes something else never sends them. One test per resource, each with
// the platform holding a non-default value.

func neverSent(f *fakefacade.Fake, key string, names ...string) func(*terraform.State) error {
	return func(*terraform.State) error {
		body := f.LastBody("PUT", key)
		for _, n := range names {
			if _, sent := body[n]; sent {
				return fmt.Errorf("an unstated %s was sent to %s: %v", n, key, body)
			}
		}
		return nil
	}
}

func TestUpgradeSLIFieldsAreLeftAloneByAConfigurationThatNeverStatedThem(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	fields := []string{"good_events_query", "total_events_query", "threshold_good", "unit"}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: sliConfig(f.URL, "")},
			{
				PreConfig: func() {
					f.Mutate("slis", mustSLIID(t, f), func(r map[string]any) {
						r["good_events_query"] = "sum(rate(ok[5m]))"
						r["total_events_query"] = "sum(rate(all[5m]))"
						r["threshold_good"] = 99.5
						r["unit"] = "ms"
					})
				},
				Config:           sliConfig(f.URL, ""),
				ConfigPlanChecks: emptyPlan,
			},
			{
				// Something else changes: the four stay out of the call and keep their values.
				Config: providerBlock(f.URL) + "resource \"sreagent_sli\" \"s\" {\n  name = \"checkout v2\"\n  sli_type = \"availability\"\n}\n",
				Check: resource.ComposeTestCheckFunc(
					neverSent(f, "slis", fields...),
					resource.TestCheckResourceAttr(sliAddr, "threshold_good", "99.5"),
					resource.TestCheckResourceAttr(sliAddr, "unit", "ms"),
					resource.TestCheckResourceAttr(sliAddr, "good_events_query", "sum(rate(ok[5m]))"),
				),
			},
		},
	})
}

func TestUpgradeOutboundConfigCardEventFieldsAreLeftAloneByAConfigurationThatNeverStatedThem(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: outboundConfigConfig(f.URL, "")},
			{
				PreConfig: func() {
					for _, id := range f.IDs("outbound_configs") {
						f.Mutate("outbound_configs", id, func(r map[string]any) {
							r["card_event_kinds"] = []any{"moved", "comments"}
							r["card_event_text"] = true
						})
					}
				},
				Config:           outboundConfigConfig(f.URL, ""),
				ConfigPlanChecks: emptyPlan,
			},
			{
				Config: outboundConfigConfig(f.URL, "  enabled = false"),
				Check: resource.ComposeTestCheckFunc(
					neverSent(f, "outbound_configs", "card_event_kinds", "card_event_text"),
					resource.TestCheckResourceAttr(outboundConfig, "card_event_text", "true"),
					resource.TestCheckResourceAttr(outboundConfig, "card_event_kinds.#", "2"),
				),
			},
		},
	})
}

func TestUpgradeBoardSummaryModelEnabledIsLeftAloneByAConfigurationThatNeverStatedIt(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12")},
			{
				PreConfig: func() {
					f.Mutate("organization_settings", "organization_settings", func(r map[string]any) { r["board_summary_model_enabled"] = true })
				},
				Config:           orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				ConfigPlanChecks: emptyPlan,
			},
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"),
				Check: resource.ComposeTestCheckFunc(
					neverSent(f, "organization_settings", "board_summary_model_enabled"),
					resource.TestCheckResourceAttr(orgSettings, "board_summary_model_enabled", "true"),
				),
			},
		},
	})
}
