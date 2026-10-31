package provider

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

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
