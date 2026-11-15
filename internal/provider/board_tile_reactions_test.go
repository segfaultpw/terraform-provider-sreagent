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

// board_tile_reactions is whether a board tile shows its card's reactions. The
// platform answers true before anyone chooses.
func TestBoardTileReactionsRoundTripAndAreLeftAloneWhenLeftOut(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check:  resource.TestCheckResourceAttr(orgSettings, "board_tile_reactions", "true"),
			},
			{
				Config: orgSettingsConfig(f.URL, "  board_tile_reactions = false"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "board_tile_reactions", "false"),
					func(*terraform.State) error {
						if orgRow(f)["board_tile_reactions"] != false {
							return fmt.Errorf("the platform holds %v", orgRow(f)["board_tile_reactions"])
						}
						return nil
					},
				),
			},
			{
				// Left out, the attribute keeps what the platform holds (false) and is not sent.
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 13"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "board_tile_reactions", "false"),
					func(*terraform.State) error {
						if orgPutHas(f, "board_tile_reactions") {
							return fmt.Errorf("an unset board_tile_reactions was sent: %v", f.LastBody("PUT", "organization_settings"))
						}
						return nil
					},
				),
			},
			{Config: orgSettingsConfig(f.URL, "  board_tile_reactions = true")},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: orgSettingsConfig(f.URL, "  board_tile_reactions = true")},
		},
	})
}

// A configuration that does not name the attribute plans nothing against a
// platform value of true or of false, and never sends it on an apply that
// changes something else.
func TestAConfigurationThatLeavesBoardTileReactionsOutPlansNothingWhateverThePlatformHolds(t *testing.T) {
	for _, held := range []bool{true, false} {
		t.Run(fmt.Sprint(held), func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			f.Mutate("organization_settings", "organization_settings", func(r map[string]any) { r["board_tile_reactions"] = held })
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{
					{
						Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
						Check: resource.ComposeTestCheckFunc(
							resource.TestCheckResourceAttr(orgSettings, "board_tile_reactions", fmt.Sprint(held)),
							func(*terraform.State) error {
								if orgPutHas(f, "board_tile_reactions") {
									return fmt.Errorf("board_tile_reactions was sent without being configured")
								}
								return nil
							},
						),
					},
					{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), ConfigPlanChecks: emptyPlan},
					{
						Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"),
						Check: func(*terraform.State) error {
							if orgPutHas(f, "board_tile_reactions") {
								return fmt.Errorf("board_tile_reactions was sent by an apply that did not name it")
							}
							if orgRow(f)["board_tile_reactions"] != held {
								return fmt.Errorf("the platform now holds %v, not %v", orgRow(f)["board_tile_reactions"], held)
							}
							return nil
						},
					},
				},
			})
		})
	}
}

func TestBoardTileReactionsAreReadByTheDataSourceAndLeftInPlaceOnDestroy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: orgSettingsConfig(f.URL, "  board_tile_reactions = false")},
			{
				Config: providerBlock(f.URL) + "data \"sreagent_organization_settings\" \"o\" {}\n",
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.sreagent_organization_settings.o", "board_tile_reactions", "false"),
					func(*terraform.State) error {
						if orgRow(f)["board_tile_reactions"] != false {
							return fmt.Errorf("destroying the resource changed the setting to %v", orgRow(f)["board_tile_reactions"])
						}
						return nil
					},
				),
			},
		},
	})
}

// A platform from before the attribute answers none: it reads as null and
// nothing else about the row changes.
func TestBoardTileReactionsReadAnOlderPlatformWithoutThem(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) { delete(r, "board_tile_reactions") })
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check:  resource.TestCheckNoResourceAttr(orgSettings, "board_tile_reactions"),
			},
			{Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"), ConfigPlanChecks: emptyPlan},
		},
	})
}

// A platform from before the attribute refuses it as an argument its tool does
// not declare, and the plan shows that sentence when a configuration sets it.
func TestBoardTileReactionsSetAgainstAnOlderPlatformShowThePlatformsRefusal(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) { delete(r, "board_tile_reactions") })
	f.Undeclare("organization_settings", "board_tile_reactions")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      orgSettingsConfig(f.URL, "  board_tile_reactions = false"),
			ExpectError: regexp.MustCompile(`(?s)board_tile_reactions\s+is\s+not\s+an\s+argument\s+this\s+tool\s+declares`),
		}},
	})
}
