package provider

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const orgData = "data.sreagent_organization_settings.o"

// holds checks what the platform stores for a field.
func holds(f *fakefacade.Fake, key string, want any) resource.TestCheckFunc {
	return func(*terraform.State) error {
		got, present := orgRow(f)[key]
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		if !present || string(g) != string(w) {
			return fmt.Errorf("the platform holds %s (answered: %v) for %s, want %s", g, present, key, w)
		}
		return nil
	}
}

// notSent checks that the last write did not name a field.
func notSent(f *fakefacade.Fake, key string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if orgPutHas(f, key) {
			return fmt.Errorf("%s was sent without being configured: %v", key, f.LastBody("PUT", "organization_settings"))
		}
		return nil
	}
}

// ---- looking_window_hours ----

// The window is 4, 8, 24 or 72 hours; 0 is how the configuration says until the reaction
// is removed, which the platform stores as null.
func TestLookingWindowTakesEachAllowedValueAndUntilRemoved(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(v string) string { return orgSettingsConfig(f.URL, "  looking_window_hours = "+v) }
	step := func(v string, stored any) resource.TestStep {
		return resource.TestStep{
			Config: cfg(v),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(orgSettings, "looking_window_hours", v),
				holds(f, "looking_window_hours", stored),
			),
		}
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			// The platform answers 24 before anyone chooses.
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check:  resource.TestCheckResourceAttr(orgSettings, "looking_window_hours", "24"),
			},
			step("4", float64(4)),
			{Config: cfg("4"), ConfigPlanChecks: emptyPlan},
			step("8", float64(8)),
			step("72", float64(72)),
			// Until the reaction is removed: sent as null, stored as null, read back as 0.
			{
				Config: cfg("0"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "looking_window_hours", "0"),
					holds(f, "looking_window_hours", nil),
					func(*terraform.State) error {
						if got, sent := f.LastBody("PUT", "organization_settings")["looking_window_hours"]; !sent || got != nil {
							return fmt.Errorf("until removed must be sent as null, sent %v (named: %v)", got, sent)
						}
						return nil
					},
				),
			},
			{Config: cfg("0"), ConfigPlanChecks: emptyPlan},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: cfg("0")},
			step("24", float64(24)),
			{Config: cfg("24"), ConfigPlanChecks: emptyPlan},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: cfg("24")},
		},
	})
}

// A configuration that leaves the window out plans nothing against whatever the platform
// holds (24, any other value, or null for until removed) and never sends it.
func TestAConfigurationThatLeavesTheLookingWindowOutPlansNothingWhateverThePlatformHolds(t *testing.T) {
	cases := []struct {
		name   string
		held   any
		wantIn string
	}{
		{"24", float64(24), "24"},
		{"8", float64(8), "8"},
		{"until removed", nil, "0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			f.Mutate("organization_settings", "organization_settings", func(r map[string]any) { r["looking_window_hours"] = c.held })
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{
					{
						Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
						Check: resource.ComposeTestCheckFunc(
							resource.TestCheckResourceAttr(orgSettings, "looking_window_hours", c.wantIn),
							notSent(f, "looking_window_hours"),
						),
					},
					{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), ConfigPlanChecks: emptyPlan},
					{
						Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"),
						Check: resource.ComposeTestCheckFunc(
							notSent(f, "looking_window_hours"),
							holds(f, "looking_window_hours", c.held),
						),
					},
				},
			})
		})
	}
}

// Going from until removed to a number and back is a plain update, and a number the
// platform does not offer is refused at plan, before any write.
func TestLookingWindowRefusesAnythingOffTheListAtPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	for _, bad := range []string{"1", "5", "48", "73", "-1", "168"} {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: factories,
			Steps: []resource.TestStep{{
				Config:      orgSettingsConfig(f.URL, "  looking_window_hours = "+bad),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)looking_window_hours.*must be one of 4, 8, 24, 72`),
			}},
		})
	}
	if n := f.Calls("PUT", "organization_settings"); n != 0 {
		t.Fatalf("a refused configuration still sent %d PUTs", n)
	}
}

// The data source reads the platform's null as null: it has no number to stand in.
func TestLookingWindowIsReadByTheDataSourceAndLeftInPlaceOnDestroy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: orgSettingsConfig(f.URL, "  looking_window_hours = 72")},
			{
				Config: providerBlock(f.URL) + "data \"sreagent_organization_settings\" \"o\" {}\n",
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgData, "looking_window_hours", "72"),
					holds(f, "looking_window_hours", float64(72)),
				),
			},
			{Config: orgSettingsConfig(f.URL, "  looking_window_hours = 0")},
			{
				Config: providerBlock(f.URL) + "data \"sreagent_organization_settings\" \"o\" {}\n",
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(orgData, "looking_window_hours"),
					holds(f, "looking_window_hours", nil),
				),
			},
		},
	})
}

// ---- reaction_meanings_off ----

func TestReactionMeaningsAreASetAndEmptyLeavesEveryMeaningOn(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(list string) string { return orgSettingsConfig(f.URL, "  reaction_meanings_off = "+list) }
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check:  resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.#", "0"),
			},
			{
				Config: cfg(`["take_it", "looking"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.#", "2"),
					// The platform stores the list sorted.
					holds(f, "reaction_meanings_off", []any{"looking", "take_it"}),
				),
			},
			// The order is no change.
			{Config: cfg(`["take_it", "looking"]`), ConfigPlanChecks: emptyPlan},
			{Config: cfg(`["looking", "take_it"]`), ConfigPlanChecks: emptyPlan},
			{
				Config: cfg(`["take_it"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.#", "1"),
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.0", "take_it"),
				),
			},
			{
				// Left out keeps it as it is, and sends nothing.
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.#", "1"),
					notSent(f, "reaction_meanings_off"),
				),
			},
			{
				Config: cfg(`[]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.#", "0"),
					func(*terraform.State) error {
						if got, sent := f.LastBody("PUT", "organization_settings")["reaction_meanings_off"]; !sent || !reflect.DeepEqual(got, []any{}) {
							return fmt.Errorf("an empty list must be sent as [], sent %v", f.LastBody("PUT", "organization_settings"))
						}
						return nil
					},
				),
			},
			{Config: cfg(`[]`), ConfigPlanChecks: emptyPlan},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: cfg(`[]`)},
		},
	})
}

func TestReactionMeaningsRefuseAnUnknownMeaningAndARepeatAtPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:      orgSettingsConfig(f.URL, "  reaction_meanings_off = [\"celebrate\"]"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)reaction_meanings_off.*value must be one of`),
			},
			{
				Config:      orgSettingsConfig(f.URL, "  reaction_meanings_off = [\"looking\", \"looking\"]"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`duplicate values`),
			},
		},
	})
	if n := f.Calls("PUT", "organization_settings"); n != 0 {
		t.Fatalf("a refused configuration still sent %d PUTs", n)
	}
}

func TestReactionMeaningsAreReadByTheDataSourceAndLeftInPlaceOnDestroy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: orgSettingsConfig(f.URL, "  reaction_meanings_off = [\"looking\"]")},
			{
				Config: providerBlock(f.URL) + "data \"sreagent_organization_settings\" \"o\" {}\n",
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgData, "reaction_meanings_off.#", "1"),
					resource.TestCheckResourceAttr(orgData, "reaction_meanings_off.0", "looking"),
					holds(f, "reaction_meanings_off", []any{"looking"}),
				),
			},
		},
	})
}

// ---- both ----

// A platform from before the two settings answers neither: they read as null (the window
// stays null, not 0) and nothing else about the row changes.
func TestReactionMeaningSettingsReadAnOlderPlatformWithoutThem(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		delete(r, "looking_window_hours")
		delete(r, "reaction_meanings_off")
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(orgSettings, "looking_window_hours"),
					resource.TestCheckNoResourceAttr(orgSettings, "reaction_meanings_off"),
				),
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), ConfigPlanChecks: emptyPlan},
		},
	})
}

// A configuration written for 0.5.0 names neither, and upgrading plans nothing for it.
func TestAConfigurationThatNamesNeitherReactionMeaningSettingSendsNeither(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12\n  board_tile_reactions = false"),
				Check: resource.ComposeTestCheckFunc(
					notSent(f, "looking_window_hours"),
					notSent(f, "reaction_meanings_off"),
				),
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12\n  board_tile_reactions = false"), ConfigPlanChecks: emptyPlan},
		},
	})
}
