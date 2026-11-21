package provider

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const (
	checkMark = "✅"          // check mark button
	rocket    = "\U0001F680" // rocket
	fire      = "\U0001F525" // fire
)

// columnEmojiConfig writes the map in the order of its keys, so a test's configuration is stable.
func columnEmojiConfig(url string, entries map[string]string) string {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("  board_column_emoji = {\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "    %s = %q\n", k, entries[k])
	}
	b.WriteString("  }")
	return orgSettingsConfig(url, b.String())
}

// columnEmojiIs checks the map the platform stores.
func columnEmojiIs(f *fakefacade.Fake, want map[string]string) func(*terraform.State) error {
	return func(*terraform.State) error {
		got, _ := orgRow(f)["board_column_emoji"].(map[string]any)
		w := map[string]any{}
		for k, v := range want {
			w[k] = v
		}
		if !reflect.DeepEqual(got, w) {
			return fmt.Errorf("the platform holds %q, want %q", got, w)
		}
		return nil
	}
}

func lastColumnEmojiSent(f *fakefacade.Fake) (map[string]any, bool) {
	v, sent := f.LastBody("PUT", "organization_settings")["board_column_emoji"]
	m, _ := v.(map[string]any)
	return m, sent
}

// The map is the whole set: a column the platform holds and the configuration leaves out is removed,
// {} removes them all, and the order of the keys is nothing.
func TestBoardColumnEmojiIsTheWholeSetAndEmptyClearsEveryColumn(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			// Before anyone chooses the platform answers an empty object.
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check:  resource.TestCheckResourceAttr(orgSettings, "board_column_emoji.%", "0"),
			},
			{
				Config: columnEmojiConfig(f.URL, map[string]string{"done": checkMark, "in_progress": rocket, "blocked": fire}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "board_column_emoji.%", "3"),
					resource.TestCheckResourceAttr(orgSettings, "board_column_emoji.done", checkMark),
					resource.TestCheckResourceAttr(orgSettings, "board_column_emoji.in_progress", rocket),
					columnEmojiIs(f, map[string]string{"done": checkMark, "in_progress": rocket, "blocked": fire}),
				),
			},
			{Config: columnEmojiConfig(f.URL, map[string]string{"done": checkMark, "in_progress": rocket, "blocked": fire}), ConfigPlanChecks: emptyPlan},
			{
				// A column left out is removed: the map replaces the set.
				Config: columnEmojiConfig(f.URL, map[string]string{"done": checkMark}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "board_column_emoji.%", "1"),
					columnEmojiIs(f, map[string]string{"done": checkMark}),
					func(*terraform.State) error {
						if got, sent := lastColumnEmojiSent(f); !sent || !reflect.DeepEqual(got, map[string]any{"done": checkMark}) {
							return fmt.Errorf("the update sent %v (named: %v), want the whole map", got, sent)
						}
						return nil
					},
				),
			},
			{
				// Left out keeps what the organization has and sends nothing.
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "board_column_emoji.%", "1"),
					notSent(f, "board_column_emoji"),
					columnEmojiIs(f, map[string]string{"done": checkMark}),
				),
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"), ConfigPlanChecks: emptyPlan},
			{
				Config: orgSettingsConfig(f.URL, "  board_column_emoji = {}"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "board_column_emoji.%", "0"),
					columnEmojiIs(f, nil),
					func(*terraform.State) error {
						if got, sent := lastColumnEmojiSent(f); !sent || got == nil || len(got) != 0 {
							return fmt.Errorf("an empty map must be sent as {}, sent %v (named: %v)", got, sent)
						}
						return nil
					},
				),
			},
			{Config: orgSettingsConfig(f.URL, "  board_column_emoji = {}"), ConfigPlanChecks: emptyPlan},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: orgSettingsConfig(f.URL, "  board_column_emoji = {}")},
		},
	})
}

// The platform stores the fully qualified form of an emoji written without its presentation
// selector, as it does for the quick row. The spelling in the configuration stays in state and
// plans no change, a fresh import of the platform's spelling plans nothing for it, and a change of
// the emoji itself is still a change.
func TestBoardColumnEmojiSpelledWithoutItsSelectorPlansNoChange(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: columnEmojiConfig(f.URL, map[string]string{"todo": heart, "done": checkMark}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "board_column_emoji.todo", heart),
					columnEmojiIs(f, map[string]string{"todo": heartQ, "done": checkMark}),
					func(*terraform.State) error {
						if got, _ := lastColumnEmojiSent(f); got["todo"] != heart {
							return fmt.Errorf("the write sent %q, want the configuration's %q", got["todo"], heart)
						}
						return nil
					},
				),
			},
			{Config: columnEmojiConfig(f.URL, map[string]string{"todo": heart, "done": checkMark}), ConfigPlanChecks: emptyPlan},
			{
				// The platform's own spelling of the same map is the same map.
				Config:           columnEmojiConfig(f.URL, map[string]string{"todo": heartQ, "done": checkMark}),
				ConfigPlanChecks: emptyPlan,
				Check:            noOrgPut(f, 1),
			},
			{
				Config: columnEmojiConfig(f.URL, map[string]string{"todo": fire, "done": checkMark}),
				Check:  columnEmojiIs(f, map[string]string{"todo": fire, "done": checkMark}),
			},
		},
	})
}

// A map imported from the platform holds its fully qualified spelling, and a configuration that
// writes the same emoji without the selector adopts it with no diff and no write.
func TestBoardColumnEmojiImportedInTheQualifiedFormPlansCleanForTheBareSpelling(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		r["board_column_emoji"] = map[string]any{"backlog": heartQ}
	})
	cfg := columnEmojiConfig(f.URL, map[string]string{"backlog": heart})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStatePersist: true, Config: cfg},
			{Config: cfg, ConfigPlanChecks: emptyPlan, Check: noOrgPut(f, 0)},
		},
	})
}

// Adopting a singleton whose platform already holds a map: the block's map replaces it, and a
// column the block leaves out is removed, which the apply sends whole.
func TestBoardColumnEmojiAdoptingARowReplacesWhatItHolds(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		r["board_column_emoji"] = map[string]any{"todo": fire, "done": checkMark}
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: columnEmojiConfig(f.URL, map[string]string{"done": rocket}),
				Check:  columnEmojiIs(f, map[string]string{"done": rocket}),
			},
			{Config: columnEmojiConfig(f.URL, map[string]string{"done": rocket}), ConfigPlanChecks: emptyPlan},
		},
	})
}

// What the provider can judge is refused at plan, before any write: a column the board has no such
// column for, a Slack name (the platform stores the emoji it names, which would read back as a
// different value and plan a change on every run), a key with no value, and a value over the
// platform's bounds.
func TestBoardColumnEmojiIsRefusedAtPlanWhenTheProviderCanSayWhy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:      columnEmojiConfig(f.URL, map[string]string{"review": checkMark}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Unknown\s+key.*"review"\s+is\s+not\s+one\s+of\s+the\s+keys\s+the\s+platform\s+takes:\s+backlog,\s+triage,\s+todo,\s+in_progress,\s+blocked,\s+done`),
			},
			{
				Config:      columnEmojiConfig(f.URL, map[string]string{"done": ":white_check_mark:"}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Slack\s+name.*plan\s+a\s+change\s+on\s+every\s+run.*Write\s+the\s+emoji\s+itself`),
			},
			{
				Config:      orgSettingsConfig(f.URL, "  board_column_emoji = {\n    done = null\n  }"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)A\s+key\s+with\s+no\s+value.*leave\s+"done"\s+out`),
			},
			{
				Config:      columnEmojiConfig(f.URL, map[string]string{"done": strings.Repeat("a", 65)}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Value\s+too\s+long.*at\s+most\s+64\s+characters,\s+this\s+one\s+has\s+65`),
			},
			{
				Config:      columnEmojiConfig(f.URL, map[string]string{"done": strings.Repeat("a", 17)}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Value\s+the\s+platform\s+stores\s+differently.*at\s+most\s+16\s+code\s+points\s+and\s+64\s+bytes`),
			},
		},
	})
	if n := f.Calls("PUT", "organization_settings"); n != 0 {
		t.Fatalf("a refused configuration still sent %d PUTs", n)
	}
}

// Whether a value is an emoji of the platform's set only the platform knows: its sentence reaches
// the person and nothing is stored.
func TestBoardColumnEmojiTheProviderCannotJudgeShowsThePlatformsRefusal(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      columnEmojiConfig(f.URL, map[string]string{"done": "nope"}),
			ExpectError: regexp.MustCompile(`(?s)The\s+emoji\s+for\s+done\s+is\s+not\s+an\s+emoji\s+SRE\s+Agent\s+knows`),
		}},
	})
	if got, _ := orgRow(f)["board_column_emoji"].(map[string]any); len(got) != 0 {
		t.Fatalf("a refused write stored %v", got)
	}
}

// The data source reads the map, and destroying the resource leaves it in place.
func TestBoardColumnEmojiIsReadByTheDataSourceAndLeftInPlaceOnDestroy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: columnEmojiConfig(f.URL, map[string]string{"done": checkMark, "triage": fire})},
			{
				Config: providerBlock(f.URL) + "data \"sreagent_organization_settings\" \"o\" {}\n",
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgData, "board_column_emoji.%", "2"),
					resource.TestCheckResourceAttr(orgData, "board_column_emoji.done", checkMark),
					resource.TestCheckResourceAttr(orgData, "board_column_emoji.triage", fire),
					columnEmojiIs(f, map[string]string{"done": checkMark, "triage": fire}),
				),
			},
		},
	})
}

// A platform from before the setting answers none: the map reads as null (not {}), nothing else
// changes, and setting it shows the platform's own refusal, not an inconsistent result.
func TestBoardColumnEmojiAgainstAnOlderPlatform(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) { delete(r, "board_column_emoji") })
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check:  resource.TestCheckNoResourceAttr(orgSettings, "board_column_emoji"),
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), ConfigPlanChecks: emptyPlan},
		},
	})
	g := fakefacade.New(t, specs.All())
	g.Mutate("organization_settings", "organization_settings", func(r map[string]any) { delete(r, "board_column_emoji") })
	g.Undeclare("organization_settings", "board_column_emoji")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      columnEmojiConfig(g.URL, map[string]string{"done": checkMark}),
			ExpectError: regexp.MustCompile(`(?s)board_column_emoji\s+is\s+not\s+an\s+argument\s+this\s+tool\s+declares`),
		}},
	})
}
