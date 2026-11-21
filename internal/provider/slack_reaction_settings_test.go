package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// ---- slack_reaction_sync ----

func TestSlackReactionSyncTakesEachModeAndIsLeftAloneWhenLeftOut(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(v string) string { return orgSettingsConfig(f.URL, "  slack_reaction_sync = \""+v+"\"") }
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			// The platform answers both before anyone chooses.
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "slack_reaction_sync", "both"),
					notSent(f, "slack_reaction_sync"),
				),
			},
			{
				Config: cfg("from_slack"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "slack_reaction_sync", "from_slack"),
					holds(f, "slack_reaction_sync", "from_slack"),
				),
			},
			{Config: cfg("from_slack"), ConfigPlanChecks: emptyPlan},
			{Config: cfg("off"), Check: holds(f, "slack_reaction_sync", "off")},
			{
				// Left out keeps what the organization has and sends nothing.
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "slack_reaction_sync", "off"),
					notSent(f, "slack_reaction_sync"),
				),
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"), ConfigPlanChecks: emptyPlan},
			{Config: cfg("both"), Check: holds(f, "slack_reaction_sync", "both")},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: cfg("both")},
		},
	})
}

func TestSlackReactionSyncRefusesAnyOtherModeAtPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	for _, bad := range []string{"to_slack", "Both", "none", ""} {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: factories,
			Steps: []resource.TestStep{{
				Config:      orgSettingsConfig(f.URL, "  slack_reaction_sync = \""+bad+"\""),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)slack_reaction_sync.*value\s+must\s+be\s+one\s+of.*both.*from_slack.*off`),
			}},
		})
	}
	if n := f.Calls("PUT", "organization_settings"); n != 0 {
		t.Fatalf("a refused configuration still sent %d PUTs", n)
	}
}

// ---- slack_reaction_counts ----

func TestSlackReactionCountsAreOnByDefaultAndSwitchOff(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(v string) string { return orgSettingsConfig(f.URL, "  slack_reaction_counts = "+v) }
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "slack_reaction_counts", "true"),
					notSent(f, "slack_reaction_counts"),
				),
			},
			{
				Config: cfg("false"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "slack_reaction_counts", "false"),
					holds(f, "slack_reaction_counts", false),
				),
			},
			{Config: cfg("false"), ConfigPlanChecks: emptyPlan},
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "slack_reaction_counts", "false"),
					notSent(f, "slack_reaction_counts"),
				),
			},
			{Config: cfg("true"), Check: holds(f, "slack_reaction_counts", true)},
			{Config: cfg("true"), ConfigPlanChecks: emptyPlan},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: cfg("true")},
		},
	})
}

// The data source answers both, and destroying the resource leaves them in place.
func TestSlackReactionSettingsAreReadByTheDataSourceAndLeftInPlaceOnDestroy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: orgSettingsConfig(f.URL, "  slack_reaction_sync = \"from_slack\"\n  slack_reaction_counts = false")},
			{
				Config: providerBlock(f.URL) + "data \"sreagent_organization_settings\" \"o\" {}\n",
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgData, "slack_reaction_sync", "from_slack"),
					resource.TestCheckResourceAttr(orgData, "slack_reaction_counts", "false"),
					holds(f, "slack_reaction_sync", "from_slack"),
					holds(f, "slack_reaction_counts", false),
				),
			},
		},
	})
}

// A platform from before the settings answers neither: they read as null, nothing else changes,
// and setting either shows the platform's own refusal.
func TestSlackReactionSettingsAgainstAnOlderPlatform(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		delete(r, "slack_reaction_sync")
		delete(r, "slack_reaction_counts")
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(orgSettings, "slack_reaction_sync"),
					resource.TestCheckNoResourceAttr(orgSettings, "slack_reaction_counts"),
				),
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), ConfigPlanChecks: emptyPlan},
		},
	})
	for _, c := range []struct{ attr, line string }{
		{"slack_reaction_sync", "  slack_reaction_sync = \"off\""},
		{"slack_reaction_counts", "  slack_reaction_counts = false"},
	} {
		t.Run(c.attr, func(t *testing.T) {
			g := fakefacade.New(t, specs.All())
			g.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
				delete(r, "slack_reaction_sync")
				delete(r, "slack_reaction_counts")
			})
			g.Undeclare("organization_settings", "slack_reaction_sync")
			g.Undeclare("organization_settings", "slack_reaction_counts")
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config:      orgSettingsConfig(g.URL, c.line),
					ExpectError: regexp.MustCompile(`(?s)` + c.attr + `\s+is\s+not\s+an\s+argument\s+this\s+tool\s+declares`),
				}},
			})
		})
	}
}

// A configuration written for 0.6.0 names none of the four settings 0.7.0 adds, and upgrading
// plans nothing for it whatever the platform holds, and sends none of them.
func TestAConfigurationThatNamesNoneOfTheFourSettingsAddedInZeroSevenSendsNone(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		r["board_column_emoji"] = map[string]any{"done": checkMark}
		r["slack_reaction_sync"] = "off"
		r["slack_reaction_counts"] = false
		r["reaction_meanings_off"] = []any{"tell_author", "answer_feedback"}
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: func(*terraform.State) error {
					for _, k := range []string{"board_column_emoji", "slack_reaction_sync", "slack_reaction_counts", "reaction_meanings_off"} {
						if orgPutHas(f, k) {
							return fmt.Errorf("%s was sent without being configured", k)
						}
					}
					return nil
				},
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), ConfigPlanChecks: emptyPlan},
		},
	})
}
