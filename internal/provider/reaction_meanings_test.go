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
				ExpectError: regexp.MustCompile(`(?s)looking_window_hours.*must\s+be\s+one\s+of\s+4,\s+8,\s+24,\s+72.*Use\s+0\s+to\s+show\s+who\s+is\s+looking\s+until\s+the\s+reaction\s+is\s+removed`),
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

// All four meanings are declared: tell_author (reactions tell the author of a card or comment) and
// answer_feedback (a thumbs on an assistant answer is a rating) are managed like looking and take_it,
// the platform stores the set sorted, and the order is no change.
func TestAllFourReactionMeaningsAreASetTheProviderManages(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(list string) string { return orgSettingsConfig(f.URL, "  reaction_meanings_off = "+list) }
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg(`["tell_author"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.#", "1"),
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.0", "tell_author"),
					holds(f, "reaction_meanings_off", []any{"tell_author"}),
				),
			},
			{Config: cfg(`["tell_author"]`), ConfigPlanChecks: emptyPlan},
			{
				Config: cfg(`["tell_author", "answer_feedback", "take_it", "looking"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.#", "4"),
					holds(f, "reaction_meanings_off", []any{"answer_feedback", "looking", "take_it", "tell_author"}),
				),
			},
			{Config: cfg(`["looking", "take_it", "answer_feedback", "tell_author"]`), ConfigPlanChecks: emptyPlan},
			{
				Config: cfg(`["answer_feedback"]`),
				Check:  holds(f, "reaction_meanings_off", []any{"answer_feedback"}),
			},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: cfg(`["answer_feedback"]`)},
			{Config: cfg(`[]`), Check: holds(f, "reaction_meanings_off", []any{})},
			{Config: cfg(`[]`), ConfigPlanChecks: emptyPlan},
		},
	})
}

// An organization that switched tell_author and answer_feedback off in the app while only 0.6.0
// was out: 0.6.0 kept them off without showing them. A block that names the set the 0.6.0 way,
// without them, now plans their removal (the next apply switches them back on): the upgrade note
// says to add them. A block that names them plans nothing, and one that leaves the attribute out
// plans nothing and sends nothing.
func TestUpgradingFromZeroSixShowsTheTwoMeaningsAsRemovedUnlessTheBlockNamesThem(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		r["reaction_meanings_off"] = []any{"answer_feedback", "looking", "tell_author"}
	})
	cfg := func(list string) string { return orgSettingsConfig(f.URL, "  reaction_meanings_off = "+list) }
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			// A block that leaves the attribute out plans nothing against what the platform holds.
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.#", "3"),
					notSent(f, "reaction_meanings_off"),
				),
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), ConfigPlanChecks: emptyPlan},
			// The block of a 0.6.0 configuration names looking alone: the plan now shows the two as removed.
			{
				Config:             cfg(`["looking"]`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			// Naming all three keeps them off and plans nothing.
			{Config: cfg(`["looking", "tell_author", "answer_feedback"]`), ConfigPlanChecks: emptyPlan},
			{Config: cfg(`["answer_feedback", "looking", "tell_author"]`), ConfigPlanChecks: emptyPlan, Check: holds(f, "reaction_meanings_off", []any{"answer_feedback", "looking", "tell_author"})},
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

// ---- a meaning this provider release does not know ----

// A platform release newer than this provider admits a meaning it does not declare (fakefacade.FutureMeaning). An admin can switch it off
// in the app; a block that names reaction_meanings_off must neither show it as a change nor
// switch it back on, and cannot name it (the plan refuses it).
func TestAMeaningThisReleaseDoesNotKnowStaysOffWhenTheBlockNamesTheSet(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		r["reaction_meanings_off"] = []any{fakefacade.FutureMeaning, "looking"}
	})
	cfg := func(list string) string { return orgSettingsConfig(f.URL, "  reaction_meanings_off = "+list) }
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			// Adopting the row: the block names looking only, the apply sends both.
			{
				Config: cfg(`["looking"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.#", "1"),
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.0", "looking"),
					holds(f, "reaction_meanings_off", []any{fakefacade.FutureMeaning, "looking"}),
					func(*terraform.State) error {
						got, _ := f.LastBody("PUT", "organization_settings")["reaction_meanings_off"].([]any)
						if !reflect.DeepEqual(got, []any{"looking", fakefacade.FutureMeaning}) {
							return fmt.Errorf("the apply must send looking and the meaning it does not know, sent %v", got)
						}
						return nil
					},
				),
			},
			{Config: cfg(`["looking"]`), ConfigPlanChecks: emptyPlan},
			// A change of the set keeps it off too.
			{
				Config: cfg(`["take_it", "looking"]`),
				Check:  holds(f, "reaction_meanings_off", []any{fakefacade.FutureMeaning, "looking", "take_it"}),
			},
			{Config: cfg(`["take_it", "looking"]`), ConfigPlanChecks: emptyPlan},
			{
				Config: cfg(`[]`),
				Check:  holds(f, "reaction_meanings_off", []any{fakefacade.FutureMeaning}),
			},
			{Config: cfg(`[]`), ConfigPlanChecks: emptyPlan},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: cfg(`[]`)},
		},
	})
}

// A block that leaves the attribute out sends nothing and plans nothing, whatever the
// platform holds.
func TestAConfigurationThatLeavesTheMeaningsOutSendsNothingWhenThePlatformHoldsAnUnknownOne(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		r["reaction_meanings_off"] = []any{fakefacade.FutureMeaning, "looking"}
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: resource.ComposeTestCheckFunc(
					notSent(f, "reaction_meanings_off"),
					resource.TestCheckResourceAttr(orgSettings, "reaction_meanings_off.#", "1"),
				),
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), ConfigPlanChecks: emptyPlan},
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"),
				Check: resource.ComposeTestCheckFunc(
					notSent(f, "reaction_meanings_off"),
					holds(f, "reaction_meanings_off", []any{fakefacade.FutureMeaning, "looking"}),
				),
			},
		},
	})
}

// A configuration cannot name a meaning this release does not know: it is refused at plan.
func TestAConfigurationCannotNameAMeaningThisReleaseDoesNotKnow(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      orgSettingsConfig(f.URL, "  reaction_meanings_off = [\"future_meaning\"]"),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`(?s)reaction_meanings_off.*value must be one of`),
		}},
	})
	if n := f.Calls("PUT", "organization_settings"); n != 0 {
		t.Fatalf("a refused configuration still sent %d PUTs", n)
	}
}

// ---- against a platform before v0.506.0 ----

// A platform from before the settings refuses each as an argument its tool does not declare,
// and the person reads the platform's own sentence, not an inconsistent result.
func TestSettingEitherReactionSettingAgainstAnOlderPlatformShowsThePlatformsRefusal(t *testing.T) {
	for _, c := range []struct{ attr, line string }{
		{"looking_window_hours", "  looking_window_hours = 0"},
		{"looking_window_hours", "  looking_window_hours = 8"},
		{"reaction_meanings_off", "  reaction_meanings_off = [\"looking\"]"},
	} {
		t.Run(c.line, func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
				delete(r, "looking_window_hours")
				delete(r, "reaction_meanings_off")
			})
			f.Undeclare("organization_settings", "looking_window_hours")
			f.Undeclare("organization_settings", "reaction_meanings_off")
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config:      orgSettingsConfig(f.URL, c.line),
					ExpectError: regexp.MustCompile(`(?s)` + c.attr + `\s+is\s+not\s+an\s+argument\s+this\s+tool\s+declares`),
				}},
			})
		})
	}
}
