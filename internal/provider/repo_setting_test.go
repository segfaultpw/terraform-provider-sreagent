package provider

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const repoSetting = "sreagent_repo_setting.r"

func repoSettingConfig(url, body string) string {
	return providerBlock(url) + fmt.Sprintf("resource \"sreagent_repo_setting\" \"r\" {\n  repo = \"acme/api\"\n%s\n}\n", body)
}

func repoRow(f *fakefacade.Fake) map[string]any {
	return f.Row("repo_settings", "00000000-0000-0000-0000-000000000001")
}

// A fence of several directories round-trips, path_prefix is then null, and
// moving between the list and the single directory never leaves a stale value
// behind (the platform derives each from the other).
func TestPathPrefixesRoundTripAndMirrorPathPrefix(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: repoSettingConfig(f.URL, "  path_prefixes = [\"services/api/\", \"services/web/\"]"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoSetting, "path_prefixes.#", "2"),
					resource.TestCheckResourceAttr(repoSetting, "path_prefixes.1", "services/web/"),
					resource.TestCheckNoResourceAttr(repoSetting, "path_prefix"),
					func(*terraform.State) error {
						if got := repoRow(f)["path_prefixes"]; !reflect.DeepEqual(got, []any{"services/api/", "services/web/"}) {
							return fmt.Errorf("the platform holds %v", got)
						}
						return nil
					},
				),
			},
			{
				Config: repoSettingConfig(f.URL, "  path_prefixes = [\"services/api/\"]"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoSetting, "path_prefixes.#", "1"),
					resource.TestCheckResourceAttr(repoSetting, "path_prefix", "services/api/"),
					func(*terraform.State) error {
						if _, sent := f.LastBody("PUT", "repo_settings")["path_prefix"]; sent {
							return fmt.Errorf("path_prefix was sent with path_prefixes: %v", f.LastBody("PUT", "repo_settings"))
						}
						return nil
					},
				),
			},
			{
				Config: repoSettingConfig(f.URL, "  path_prefix = \"services/web/\""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoSetting, "path_prefix", "services/web/"),
					resource.TestCheckResourceAttr(repoSetting, "path_prefixes.#", "1"),
					resource.TestCheckResourceAttr(repoSetting, "path_prefixes.0", "services/web/"),
					func(*terraform.State) error {
						if _, sent := f.LastBody("PUT", "repo_settings")["path_prefixes"]; sent {
							return fmt.Errorf("path_prefixes was sent with path_prefix: %v", f.LastBody("PUT", "repo_settings"))
						}
						return nil
					},
				),
			},
			{ResourceName: repoSetting, ImportState: true, ImportStateVerify: true, Config: repoSettingConfig(f.URL, "  path_prefix = \"services/web/\"")},
		},
	})
}

func TestPathPrefixAndPathPrefixesCannotBothBeConfigured(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      repoSettingConfig(f.URL, "  path_prefix = \"a/\"\n  path_prefixes = [\"a/\"]"),
			ExpectError: regexp.MustCompile(`Not allowed with\s+path_prefixes`),
		}},
	})
	if n := f.Calls("POST", "repo_settings"); n != 0 {
		t.Fatalf("a refused configuration still sent %d POSTs", n)
	}
}

func TestPathPrefixesAreBoundedAtPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	entries := ""
	for i := 0; i < 21; i++ {
		entries += fmt.Sprintf("\"d%d/\",", i)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      repoSettingConfig(f.URL, "  path_prefixes = ["+entries+"]"),
			ExpectError: regexp.MustCompile(`(?s)path_prefixes.*(at most 20|20)`),
		}},
	})
}

// draft_prs null means "as the installation says": removing the argument
// returns to it, and the other three booleans are plain settings.
func TestRepoSettingDraftPRsClearsAndTheNewSettingsRoundTrip(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: repoSettingConfig(f.URL, "  draft_prs = false\n  allow_ci_config = true\n  platform_fallback = false\n  check_network = true"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoSetting, "draft_prs", "false"),
					resource.TestCheckResourceAttr(repoSetting, "allow_ci_config", "true"),
					resource.TestCheckResourceAttr(repoSetting, "platform_fallback", "false"),
					resource.TestCheckResourceAttr(repoSetting, "check_network", "true"),
				),
			},
			{
				Config: repoSettingConfig(f.URL, "  allow_ci_config = true\n  platform_fallback = false\n  check_network = true"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(repoSetting, "draft_prs"),
					func(*terraform.State) error {
						body := f.LastBody("PUT", "repo_settings")
						if v, sent := body["draft_prs"]; !sent || v != nil {
							return fmt.Errorf("removing draft_prs must send null, sent %v", body)
						}
						if _, sent := body["check_network"]; sent {
							return fmt.Errorf("an unchanged check_network was resent: %v", body)
						}
						return nil
					},
				),
			},
		},
	})
}

// A platform from before the new settings answers none of them: they read as
// null and the rest of the row is unaffected.
func TestRepoSettingReadsAnOlderPlatformWithoutTheNewSettings(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.LegacyAnswers("repo_settings")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  service = \"api\"")},
			{
				PreConfig: func() {
					f.Mutate("repo_settings", "00000000-0000-0000-0000-000000000001", func(r map[string]any) {
						for _, k := range []string{"path_prefixes", "allow_ci_config", "platform_fallback", "draft_prs", "check_network"} {
							delete(r, k)
						}
					})
				},
				Config: repoSettingConfig(f.URL, "  service = \"api\""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoSetting, "service", "api"),
					resource.TestCheckNoResourceAttr(repoSetting, "path_prefixes"),
					resource.TestCheckNoResourceAttr(repoSetting, "check_network"),
				),
			},
		},
	})
}

func TestGithubSettingsAnswersWhyTheInstallationIsStopped(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("github_settings", "github_settings", func(r map[string]any) {
		r["stopped_from"] = "11111111-1111-1111-1111-111111111111"
		r["stopped_note"] = "This organization stopped using Parent's GitHub App."
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_github_settings\" \"g\" {}\n",
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.sreagent_github_settings.g", "stopped_from", "11111111-1111-1111-1111-111111111111"),
				resource.TestCheckResourceAttrSet("data.sreagent_github_settings.g", "stopped_note"),
			),
		}},
	})
}

func TestOutboundRuleAnswersWhyItIsNotDelivering(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock(f.URL) + "resource \"sreagent_outbound_rule\" \"o\" {\n  name = \"page\"\n  outbound_config_id = \"cfg-1\"\n}\n",
				Check:  resource.TestCheckResourceAttrSet("sreagent_outbound_rule.o", "id"),
			},
			{
				PreConfig: func() {
					f.Mutate("outbound_rules", "00000000-0000-0000-0000-000000000001", func(r map[string]any) {
						r["not_delivering"] = "this organization stopped using Parent's paging targets"
					})
				},
				Config: providerBlock(f.URL) + "resource \"sreagent_outbound_rule\" \"o\" {\n  name = \"page\"\n  outbound_config_id = \"cfg-1\"\n}\n",
				Check:  resource.TestCheckResourceAttr("sreagent_outbound_rule.o", "not_delivering", "this organization stopped using Parent's paging targets"),
			},
		},
	})
}

func noPut(f *fakefacade.Fake) func(*terraform.State) error {
	return func(*terraform.State) error {
		if n := f.Calls("PUT", "repo_settings"); n != 0 {
			return fmt.Errorf("%d PUTs were sent, last body %v", n, f.LastBody("PUT", "repo_settings"))
		}
		return nil
	}
}

func putBodyIs(f *fakefacade.Fake, want map[string]any) func(*terraform.State) error {
	return func(*terraform.State) error {
		if got := f.LastBody("PUT", "repo_settings"); !reflect.DeepEqual(got, want) {
			return fmt.Errorf("the last PUT was %v, want %v", got, want)
		}
		return nil
	}
}

var emptyPlan = resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}

// follows_default is how a setting a row holds goes back on the organization
// default. The write carries only that argument, the next read says the row no
// longer holds the setting, and the plan after it is empty: the write-only list
// is never a diff by itself.
func TestFollowsDefaultPutsASetSettingBackOnTheDefault(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: repoSettingConfig(f.URL, "  check_network = true"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoSetting, "check_network", "true"),
					resource.TestCheckResourceAttr(repoSetting, "set_here.#", "1"),
					resource.TestCheckResourceAttr(repoSetting, "set_here.0", "check_network"),
					resource.TestCheckResourceAttr(repoSetting, "following.#", "5"),
				),
			},
			{
				Config: repoSettingConfig(f.URL, "  follows_default = [\"check_network\"]"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(repoSetting, plancheck.ResourceActionUpdate),
					plancheck.ExpectUnknownValue(repoSetting, tfjsonpath.New("check_network")),
				}},
				Check: resource.ComposeTestCheckFunc(
					putBodyIs(f, map[string]any{"follows_default": []any{"check_network"}}),
					resource.TestCheckResourceAttr(repoSetting, "check_network", "false"),
					resource.TestCheckResourceAttr(repoSetting, "set_here.#", "0"),
					resource.TestCheckResourceAttr(repoSetting, "following.#", "6"),
					resource.TestCheckNoResourceAttr(repoSetting, "follows_default"),
				),
			},
			{Config: repoSettingConfig(f.URL, "  follows_default = [\"check_network\"]"), ConfigPlanChecks: emptyPlan},
		},
	})
}

// A setting the row already follows is no change: no write is sent, and an
// import of such a row (the exported file names every followed setting) plans
// nothing for the list.
func TestFollowsDefaultOnAFollowingRowPlansNothing(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	all := "  follows_default = [\"fix_runner\", \"platform_fallback\", \"auto_dispatch\", \"draft_prs\", \"check_network\", \"allow_ci_config\"]"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, all), Check: resource.ComposeTestCheckFunc(
				noPut(f),
				resource.TestCheckResourceAttr(repoSetting, "set_here.#", "0"),
				resource.TestCheckResourceAttr(repoSetting, "following.#", "6"),
				resource.TestCheckResourceAttr(repoSetting, "fix_runner", "platform"),
			)},
			{Config: repoSettingConfig(f.URL, all+"\n  service = \"api\""), Check: func(*terraform.State) error {
				if body := f.LastBody("PUT", "repo_settings"); !reflect.DeepEqual(body, map[string]any{"service": "api"}) {
					return fmt.Errorf("an update that names settings the row follows sent %v", body)
				}
				return nil
			}},
		},
	})
}

// fix_runner, platform_fallback and auto_dispatch belong to the whole
// repository. Resetting one on the all-branches block clears it on the branch
// block too, and the branch block, which leaves it out, follows along with no
// diff.
func TestARepositoryWideResetMovesTheOtherBlocksWithoutADiff(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(runner string) string {
		return providerBlock(f.URL) + fmt.Sprintf(`
resource "sreagent_repo_setting" "all" {
  repo = "acme/api"
%s
}

resource "sreagent_repo_setting" "main" {
  repo       = "acme/api"
  branch     = "main"
  depends_on = [sreagent_repo_setting.all]
}
`, runner)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg("  fix_runner = \"self_hosted\""), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("sreagent_repo_setting.main", "fix_runner", "self_hosted"),
				resource.TestCheckResourceAttr("sreagent_repo_setting.main", "set_here.0", "fix_runner"),
			)},
			{Config: cfg("  follows_default = [\"fix_runner\"]"), Check: resource.ComposeTestCheckFunc(
				putBodyIs(f, map[string]any{"follows_default": []any{"fix_runner"}}),
				resource.TestCheckResourceAttr("sreagent_repo_setting.all", "fix_runner", "platform"),
			)},
			// The branch block was not touched by the apply; its refresh finds the
			// runner back on the default, and that is no change to plan.
			{Config: cfg("  follows_default = [\"fix_runner\"]"), ConfigPlanChecks: emptyPlan, Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("sreagent_repo_setting.main", "fix_runner", "platform"),
				resource.TestCheckResourceAttr("sreagent_repo_setting.main", "set_here.#", "0"),
			)},
		},
	})
}

func TestFollowsDefaultNamingAStatedSettingIsRefused(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      repoSettingConfig(f.URL, "  check_network = true\n  follows_default = [\"check_network\"]"),
			ExpectError: regexp.MustCompile(`(?s)check_network is both set and reset to follow the\s+default; send one`),
		}},
	})
	if n := f.Calls("POST", "repo_settings"); n != 0 {
		t.Fatalf("a refused configuration still sent %d POSTs", n)
	}
}

func TestFollowsDefaultRefusesAnUnknownSetting(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      repoSettingConfig(f.URL, "  follows_default = [\"branch\"]"),
			ExpectError: regexp.MustCompile(`(?s)follows_default.*one of`),
		}},
	})
}

// Upgrading from 0.1.x: a configuration that restates the value a row follows
// (the release B backfill made those follow) plans nothing and writes nothing,
// so the row keeps following.
func TestAStatedValueEqualToWhatTheRowFollowsIsNotPinned(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  service = \"api\""), Check: resource.TestCheckResourceAttr(repoSetting, "set_here.#", "0")},
			{
				Config: repoSettingConfig(f.URL, "  service = \"api\"\n  check_network = false\n  allow_ci_config = false\n  platform_fallback = true\n  fix_runner = \"platform\"\n  auto_dispatch = true"),
				Check: resource.ComposeTestCheckFunc(
					noPut(f),
					resource.TestCheckResourceAttr(repoSetting, "set_here.#", "0"),
					resource.TestCheckResourceAttr(repoSetting, "following.#", "6"),
				),
				ConfigPlanChecks: emptyPlan,
			},
		},
	})
}

// A value that differs from what the row reads is written, which makes it the
// row's own, as in 0.1.x.
func TestAStatedValueThatDiffersFromTheDefaultIsWrittenAndHeld(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  service = \"api\"")},
			{Config: repoSettingConfig(f.URL, "  service = \"api\"\n  check_network = true"), Check: resource.ComposeTestCheckFunc(
				putBodyIs(f, map[string]any{"check_network": true}),
				resource.TestCheckResourceAttr(repoSetting, "set_here.0", "check_network"),
			)},
		},
	})
}

// Under an organization default that sets draft_prs, a row that follows reads
// the default's value; the attribute stays null so a configuration that leaves
// it out plans clean, where 0.1.3 read true and planned a change forever.
func TestRepoSettingFollowingDraftPRsUnderADefaultPlansClean(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetDefaults(map[string]any{"draft_prs": true})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  service = \"api\""), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckNoResourceAttr(repoSetting, "draft_prs"),
				resource.TestCheckResourceAttr(repoSetting, "following.#", "6"),
				func(*terraform.State) error {
					if got := f.Row("repo_settings", "00000000-0000-0000-0000-000000000001")["draft_prs"]; got != nil {
						return fmt.Errorf("the row holds draft_prs %v", got)
					}
					return nil
				},
			)},
			{Config: repoSettingConfig(f.URL, "  service = \"api\""), ConfigPlanChecks: emptyPlan, Check: noPut(f)},
			// The default changes in the app: a following row has nothing to update.
			{PreConfig: func() { f.SetDefaults(map[string]any{"draft_prs": false}) }, Config: repoSettingConfig(f.URL, "  service = \"api\""), ConfigPlanChecks: emptyPlan, Check: noPut(f)},
		},
	})
}

// A row that holds draft_prs shows it, and removing it returns the row to the
// default in one write.
func TestRepoSettingHeldDraftPRsIsShownAndCleared(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetDefaults(map[string]any{"draft_prs": true})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  draft_prs = false"), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(repoSetting, "draft_prs", "false"),
				resource.TestCheckResourceAttr(repoSetting, "set_here.0", "draft_prs"),
			)},
			{Config: repoSettingConfig(f.URL, "  follows_default = [\"draft_prs\"]"), Check: resource.ComposeTestCheckFunc(
				putBodyIs(f, map[string]any{"draft_prs": nil}),
				resource.TestCheckNoResourceAttr(repoSetting, "draft_prs"),
				resource.TestCheckResourceAttr(repoSetting, "set_here.#", "0"),
			)},
		},
	})
}

// The exported file names, for a row that holds nothing, every setting in
// follows_default. Importing it plans nothing, and the plan after the import is
// empty, with draft_prs null under a default that sets it.
func TestImportOfAFollowingRowPlansClean(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetDefaults(map[string]any{"draft_prs": true, "check_network": false})
	id := seedOne(t, f.URL, "repo_settings", map[string]any{"repo": "acme/api", "service": "api"})
	block := `
resource "sreagent_repo_setting" "api" {
  repo            = "acme/api"
  service         = "api"
  follows_default = ["fix_runner", "platform_fallback", "auto_dispatch", "draft_prs", "check_network", "allow_ci_config"]
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []resource.TestStep{
			{
				Config:           providerBlock(f.URL) + block + fmt.Sprintf("\nimport {\n  to = sreagent_repo_setting.api\n  id = %q\n}\n", id),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{importsOnly{}}},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr("sreagent_repo_setting.api", "draft_prs"),
					resource.TestCheckResourceAttr("sreagent_repo_setting.api", "following.#", "6"),
					noPut(f),
				),
			},
			{Config: providerBlock(f.URL) + block, ConfigPlanChecks: emptyPlan},
		},
	})
}

// A branch row copies its repository's runner, so a branch block that puts the
// runner back on the default is followed up by one write after the create (the
// create itself never carries follows_default), which clears it on the
// repository's other rows too.
func TestABranchCreateFollowsUpOnACopiedRepositoryWideSetting(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	seedOne(t, f.URL, "repo_settings", map[string]any{"repo": "acme/api", "fix_runner": "self_hosted"})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: repoSettingConfig(f.URL, "  branch = \"main\"\n  follows_default = [\"fix_runner\"]"),
			Check: resource.ComposeTestCheckFunc(
				func(*terraform.State) error {
					if n := f.Calls("PUT", "repo_settings"); n != 1 {
						return fmt.Errorf("want one follow-up PUT, got %d", n)
					}
					if _, sent := f.LastBody("POST", "repo_settings")["follows_default"]; sent {
						return fmt.Errorf("the create carried follows_default: %v", f.LastBody("POST", "repo_settings"))
					}
					return nil
				},
				putBodyIs(f, map[string]any{"follows_default": []any{"fix_runner"}}),
				resource.TestCheckResourceAttr(repoSetting, "fix_runner", "platform"),
				resource.TestCheckResourceAttr(repoSetting, "set_here.#", "0"),
				func(*terraform.State) error {
					if held := f.Row("repo_settings", "00000000-0000-0000-0000-000000000001")["set_here"]; !reflect.DeepEqual(held, []any{}) {
						return fmt.Errorf("the repository's other row still holds %v", held)
					}
					return nil
				},
			),
		}},
	})
}

// A platform from before the repository defaults answers no set_here, so
// nothing is ever planned or sent for follows_default.
func TestFollowsDefaultOnAPlatformWithoutSetHereSendsNothing(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.LegacyAnswers("repo_settings")
	cfg := func(body string) string {
		return repoSettingConfig(f.URL, body+"\n  follows_default = [\"check_network\", \"fix_runner\"]")
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg("  service = \"api\""), Check: resource.ComposeTestCheckFunc(
				noPut(f),
				resource.TestCheckNoResourceAttr(repoSetting, "set_here.#"),
				resource.TestCheckNoResourceAttr(repoSetting, "following.#"),
			)},
			{Config: cfg("  service = \"web\""), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(repoSetting, plancheck.ResourceActionUpdate)}}, Check: putBodyIs(f, map[string]any{"service": "web"})},
		},
	})
}
