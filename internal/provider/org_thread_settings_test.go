package provider

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

func orgRow(f *fakefacade.Fake) map[string]any {
	return f.Row("organization_settings", "organization_settings")
}

func orgPutHas(f *fakefacade.Fake, key string) bool {
	_, sent := f.LastBody("PUT", "organization_settings")[key]
	return sent
}

// ---- language ----

func TestLanguageRoundTripsAndIsLeftAloneWhenLeftOut(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				// The platform answers en before anyone chooses.
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check:  resource.TestCheckResourceAttr(orgSettings, "language", "en"),
			},
			{
				Config: orgSettingsConfig(f.URL, "  language = \"es\""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "language", "es"),
					func(*terraform.State) error {
						if orgRow(f)["language"] != "es" {
							return fmt.Errorf("the platform holds %v", orgRow(f)["language"])
						}
						return nil
					},
				),
			},
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 13"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "language", "es"),
					func(*terraform.State) error {
						if orgPutHas(f, "language") {
							return fmt.Errorf("an unset language was sent: %v", f.LastBody("PUT", "organization_settings"))
						}
						return nil
					},
				),
			},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 13")},
		},
	})
}

func TestLanguageRefusesAValueTheContractDoesNotList(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      orgSettingsConfig(f.URL, "  language = \"fr\""),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`(?s)language.*value must be one of.*"en".*"es"`),
		}},
	})
}

// ---- slack_thread_events_off ----

func TestSlackThreadEventsAreASetAndEmptyPostsEverything(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(list string) string { return orgSettingsConfig(f.URL, "  slack_thread_events_off = "+list) }
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg(`["comments", "column_moves"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "slack_thread_events_off.#", "2"),
					func(*terraform.State) error {
						// The platform stores the list sorted.
						if got := orgRow(f)["slack_thread_events_off"]; !reflect.DeepEqual(got, []any{"column_moves", "comments"}) {
							return fmt.Errorf("the platform holds %v", got)
						}
						return nil
					},
				),
			},
			// The order is no change: the platform's sorted answer plans nothing.
			{Config: cfg(`["comments", "column_moves"]`), ConfigPlanChecks: emptyPlan},
			{Config: cfg(`["column_moves", "comments"]`), ConfigPlanChecks: emptyPlan},
			{
				Config: cfg(`["card_done"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "slack_thread_events_off.#", "1"),
					resource.TestCheckResourceAttr(orgSettings, "slack_thread_events_off.0", "card_done"),
				),
			},
			{
				// Left out leaves it as it is.
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "slack_thread_events_off.#", "1"),
					func(*terraform.State) error {
						if orgPutHas(f, "slack_thread_events_off") {
							return fmt.Errorf("an unset list was sent")
						}
						return nil
					},
				),
			},
			{
				// An empty list posts everything again.
				Config: cfg(`[]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "slack_thread_events_off.#", "0"),
					func(*terraform.State) error {
						if got, sent := f.LastBody("PUT", "organization_settings")["slack_thread_events_off"]; !sent || !reflect.DeepEqual(got, []any{}) {
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

func TestSlackThreadEventsRefuseAnUnknownEventAndARepeatAtPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:      orgSettingsConfig(f.URL, "  slack_thread_events_off = [\"fix_failed\"]"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)slack_thread_events_off.*value must be one of`),
			},
			{
				Config:      orgSettingsConfig(f.URL, "  slack_thread_events_off = [\"comments\", \"comments\"]"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`duplicate values`),
			},
		},
	})
	if n := f.Calls("PUT", "organization_settings"); n != 0 {
		t.Fatalf("a refused configuration still sent %d PUTs", n)
	}
}

// ---- reaction_quick_row ----

const (
	heart       = "\u2764"                                                     // unqualified: no presentation selector
	heartQ      = "\u2764\ufe0f"                                               // fully qualified
	thumbsMed   = "\U0001F44D\U0001F3FD"                                       // thumbs up, medium skin tone
	family      = "\U0001F468\u200d\U0001F469\u200d\U0001F467\u200d\U0001F466" // ZWJ sequence
	rainbowFlag = "\U0001F3F3\ufe0f\u200d\U0001F308"                           // selector inside a ZWJ sequence
	technoWoman = "\U0001F469\U0001F3FE\u200d\U0001F4BB"                       // skin tone inside a ZWJ sequence
)

func quickRowConfig(url string, entries ...string) string {
	list := ""
	for i, e := range entries {
		if i > 0 {
			list += ", "
		}
		list += fmt.Sprintf("%q", e)
	}
	return orgSettingsConfig(url, "  reaction_quick_row = ["+list+"]")
}

func quickRowIs(f *fakefacade.Fake, want ...string) func(*terraform.State) error {
	return func(*terraform.State) error {
		got, _ := orgRow(f)["reaction_quick_row"].([]any)
		w := make([]any, 0, len(want))
		for _, e := range want {
			w = append(w, e)
		}
		if !reflect.DeepEqual(got, w) {
			return fmt.Errorf("the platform holds %q, want %q", got, w)
		}
		return nil
	}
}

// A ZWJ sequence, a skin tone and a flag go to the platform and back unchanged,
// in the order written, and nothing about them plans a change.
func TestQuickRowKeepsEmojiSequencesAndTheirOrder(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: quickRowConfig(f.URL, family, thumbsMed, rainbowFlag, technoWoman),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_quick_row.#", "4"),
					resource.TestCheckResourceAttr(orgSettings, "reaction_quick_row.0", family),
					resource.TestCheckResourceAttr(orgSettings, "reaction_quick_row.1", thumbsMed),
					resource.TestCheckResourceAttr(orgSettings, "reaction_quick_row.2", rainbowFlag),
					resource.TestCheckResourceAttr(orgSettings, "reaction_quick_row.3", technoWoman),
					quickRowIs(f, family, thumbsMed, rainbowFlag, technoWoman),
				),
			},
			{Config: quickRowConfig(f.URL, family, thumbsMed, rainbowFlag, technoWoman), ConfigPlanChecks: emptyPlan},
			{
				// The order is the row's order: swapping two entries is a change, sent whole.
				Config: quickRowConfig(f.URL, thumbsMed, family, rainbowFlag, technoWoman),
				Check: resource.ComposeTestCheckFunc(
					quickRowIs(f, thumbsMed, family, rainbowFlag, technoWoman),
					func(*terraform.State) error {
						if got := f.LastBody("PUT", "organization_settings")["reaction_quick_row"]; !reflect.DeepEqual(got, []any{thumbsMed, family, rainbowFlag, technoWoman}) {
							return fmt.Errorf("the update sent %q", got)
						}
						return nil
					},
				),
			},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: quickRowConfig(f.URL, thumbsMed, family, rainbowFlag, technoWoman)},
		},
	})
}

// The platform stores the fully qualified form of an emoji written without its
// presentation selector. The spelling in the configuration stays in state, plans
// no change on the next run, and a fresh import of the platform's spelling plans
// nothing either.
func TestQuickRowSpelledWithoutItsSelectorPlansNoChange(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: quickRowConfig(f.URL, heart, thumbsMed),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_quick_row.0", heart),
					quickRowIs(f, heartQ, thumbsMed),
				),
			},
			{Config: quickRowConfig(f.URL, heart, thumbsMed), ConfigPlanChecks: emptyPlan},
			{
				// The platform's own spelling of the same row is the same row.
				Config:           quickRowConfig(f.URL, heartQ, thumbsMed),
				ConfigPlanChecks: emptyPlan,
				Check:            noOrgPut(f, 1),
			},
		},
	})
}

func noOrgPut(f *fakefacade.Fake, writes int) func(*terraform.State) error {
	return func(*terraform.State) error {
		if n := f.Calls("PUT", "organization_settings"); n != writes {
			return fmt.Errorf("%d PUTs were sent, want %d", n, writes)
		}
		return nil
	}
}

// A row imported from the platform holds its fully qualified spelling, and a
// configuration that writes the same emoji without the selector adopts it with no
// diff.
func TestQuickRowImportedInTheQualifiedFormPlansCleanForTheBareSpelling(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		r["reaction_quick_row"] = []any{heartQ}
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStatePersist: true, Config: quickRowConfig(f.URL, heart)},
			{Config: quickRowConfig(f.URL, heart), ConfigPlanChecks: emptyPlan, Check: noOrgPut(f, 0)},
		},
	})
}

func TestQuickRowEmptyUsesThePlatformsTwelveAndClearsTheRow(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), Check: resource.TestCheckResourceAttr(orgSettings, "reaction_quick_row.#", "0")},
			{Config: quickRowConfig(f.URL, "\U0001F680"), Check: quickRowIs(f, "\U0001F680")},
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 13"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_quick_row.#", "1"),
					func(*terraform.State) error {
						if orgPutHas(f, "reaction_quick_row") {
							return fmt.Errorf("an unset row was sent")
						}
						return nil
					},
				),
			},
			{
				Config: orgSettingsConfig(f.URL, "  reaction_quick_row = []"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "reaction_quick_row.#", "0"),
					quickRowIs(f),
				),
			},
			{Config: orgSettingsConfig(f.URL, "  reaction_quick_row = []"), ConfigPlanChecks: emptyPlan},
		},
	})
}

func TestQuickRowIsRefusedAtPlanWhenTheProviderCanSayWhy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	thirteen := make([]string, 13)
	for i := range thirteen {
		thirteen[i] = fmt.Sprintf("%c", rune(0x1F600+i))
	}
	long := ""
	for range 65 {
		long += "a"
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: quickRowConfig(f.URL, thirteen...), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)reaction_quick_row.*(at most 12|12 elements)`)},
			{Config: quickRowConfig(f.URL, long), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)Entry\s+too\s+long.*at\s+most\s+64\s+characters,\s+this\s+one\s+has\s+65`)},
			{Config: quickRowConfig(f.URL, strings.Repeat("a", 17)), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)Entry\s+the\s+platform\s+stores\s+differently.*at\s+most\s+16\s+code\s+points\s+and\s+64\s+bytes`)},
			{Config: quickRowConfig(f.URL, ":tada:"), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)Slack\s+name.*plan\s+a\s+change\s+on\s+every\s+run.*Write\s+the\s+emoji\s+itself`)},
			{Config: quickRowConfig(f.URL, heart, heartQ), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)same\s+value\s+twice.*once\s+the\s+platform\s+normalizes`)},
			{Config: quickRowConfig(f.URL, "\U0001F680", "\U0001F680"), PlanOnly: true, ExpectError: regexp.MustCompile(`duplicate values`)},
		},
	})
	if n := f.Calls("PUT", "organization_settings"); n != 0 {
		t.Fatalf("a refused configuration still sent %d PUTs", n)
	}
}

// What only the platform can judge (is it an emoji of its set) comes back as a
// diagnostic carrying the platform's own sentence, and nothing is stored.
func TestQuickRowTheProviderCannotJudgeShowsThePlatformsRefusal(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      quickRowConfig(f.URL, "not an emoji"),
			ExpectError: regexp.MustCompile(`(?s)up\s+to\s+12\s+different\s+emoji,\s+each\s+one\s+emoji\s+or\s+its\s+Slack\s+name`),
		}},
	})
	if got, _ := orgRow(f)["reaction_quick_row"].([]any); len(got) != 0 {
		t.Fatalf("a refused write stored %v", got)
	}
}

func TestQuickRowSendsTheConfigurationNotTheNormalizedForm(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: quickRowConfig(f.URL, heart),
			Check: func(*terraform.State) error {
				if got := f.LastBody("PUT", "organization_settings")["reaction_quick_row"]; !reflect.DeepEqual(got, []any{heart}) {
					return fmt.Errorf("the write sent %q, want the configuration's %q", got, heart)
				}
				return nil
			},
		}},
	})
}

// ---- all of it together ----

func TestThreadSettingsAreReadByTheDataSourceAndLeftInPlaceOnDestroy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := orgSettingsConfig(f.URL, "  language = \"es\"\n  slack_thread_events_off = [\"comments\"]\n  reaction_quick_row = [\"\\U0001F680\"]")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config: providerBlock(f.URL) + "data \"sreagent_organization_settings\" \"o\" {}\n",
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.sreagent_organization_settings.o", "language", "es"),
					resource.TestCheckResourceAttr("data.sreagent_organization_settings.o", "slack_thread_events_off.0", "comments"),
					resource.TestCheckResourceAttr("data.sreagent_organization_settings.o", "reaction_quick_row.0", "\U0001F680"),
					func(*terraform.State) error {
						if orgRow(f)["language"] != "es" {
							return fmt.Errorf("destroying the resource changed the language to %v", orgRow(f)["language"])
						}
						return nil
					},
				),
			},
		},
	})
}

// A platform from before the three settings answers none of them: they read as
// null and nothing else about the row changes.
func TestThreadSettingsReadAnOlderPlatformWithoutThem(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		delete(r, "language")
		delete(r, "slack_thread_events_off")
		delete(r, "reaction_quick_row")
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(orgSettings, "language"),
					resource.TestCheckNoResourceAttr(orgSettings, "slack_thread_events_off"),
					resource.TestCheckNoResourceAttr(orgSettings, "reaction_quick_row"),
				),
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), ConfigPlanChecks: emptyPlan},
		},
	})
}

// A configuration written for 0.3.0 names none of the three, and upgrading plans
// nothing for it.
func TestAConfigurationThatNamesNoneOfTheThreeSendsNoneOfThem(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: func(*terraform.State) error {
					for _, k := range []string{"language", "slack_thread_events_off", "reaction_quick_row"} {
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
