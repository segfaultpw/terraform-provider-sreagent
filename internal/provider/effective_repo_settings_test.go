package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const effective = "data.sreagent_effective_repo_settings.all"

func effectiveConfig(url, args string) string {
	return providerBlock(url) + fmt.Sprintf("data \"sreagent_effective_repo_settings\" \"all\" {\n%s\n}\n", args)
}

func repoNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("acme/service-%04d", i)
	}
	return out
}

// Every page is read: 1,200 repositories come in three requests of at most 500,
// each carrying the cursor the previous page answered.
func TestEffectiveRepoSettingsReadsEveryPage(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Reachable(repoNames(1200)...)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config: effectiveConfig(f.URL, ""),
			Check: tfresource.ComposeTestCheckFunc(
				tfresource.TestCheckResourceAttr(effective, "items.#", "1200"),
				tfresource.TestCheckResourceAttr(effective, "truncated", "false"),
				tfresource.TestCheckResourceAttr(effective, "items.0.repo", "acme/service-0000"),
				tfresource.TestCheckResourceAttr(effective, "items.1199.repo", "acme/service-1199"),
				func(*terraform.State) error {
					qs := f.Queries()
					if len(qs) == 0 || len(qs)%3 != 0 {
						return fmt.Errorf("a read of 1,200 rows made %d requests", len(qs))
					}
					for i, q := range qs[:3] {
						if q["limit"] != "500" {
							return fmt.Errorf("page %d asked for limit %q", i+1, q["limit"])
						}
						if (i == 0) != (q["cursor"] == "") {
							return fmt.Errorf("page %d carried cursor %q", i+1, q["cursor"])
						}
					}
					if qs[1]["cursor"] == qs[2]["cursor"] {
						return fmt.Errorf("the third page reused the second page's cursor")
					}
					return nil
				},
			),
		}},
	})
}

func TestEffectiveRepoSettingsSendsItsFilters(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Reachable("acme/infra-a", "acme/my-infra", "acme/web", "acme/old")
	f.Archived("acme/old")
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{
			{
				Config: effectiveConfig(f.URL, "  name = \"infra-*\"\n  runner = \"platform\"\n  has_fence = false"),
				Check: tfresource.ComposeTestCheckFunc(
					tfresource.TestCheckResourceAttr(effective, "items.#", "1"),
					tfresource.TestCheckResourceAttr(effective, "items.0.repo", "acme/infra-a"),
					func(*terraform.State) error {
						q := f.Queries()[0]
						if q["name"] != "infra-*" || q["runner"] != "platform" || q["has_fence"] != "false" {
							return fmt.Errorf("the filters were not sent: %v", q)
						}
						if _, sent := q["include_archived"]; sent {
							return fmt.Errorf("include_archived was sent without being set: %v", q)
						}
						return nil
					},
				),
			},
			{
				Config: effectiveConfig(f.URL, "  include_archived = true"),
				Check:  tfresource.TestCheckResourceAttr(effective, "items.#", "4"),
			},
			{
				Config: effectiveConfig(f.URL, ""),
				Check:  tfresource.TestCheckResourceAttr(effective, "items.#", "3"),
			},
		},
	})
}

func TestEffectiveRepoSettingsAnswersTheDefaultsAndCounts(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetDefaults(map[string]any{"auto_dispatch": false})
	f.Reachable("acme/web")
	seedOne(t, f.URL, "repo_settings", map[string]any{"repo": "acme/api", "check_network": true})
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config: effectiveConfig(f.URL, ""),
			Check: tfresource.ComposeTestCheckFunc(
				tfresource.TestCheckResourceAttr(effective, "runner_registered", "false"),
				tfresource.TestCheckResourceAttr(effective, "archived_known", "true"),
				tfresource.TestCheckResourceAttrWith(effective, "defaults", func(v string) error {
					if !strings.Contains(v, `"auto_dispatch":false`) && !strings.Contains(v, `"auto_dispatch": false`) {
						return fmt.Errorf("the defaults are %s", v)
					}
					return nil
				}),
				tfresource.TestCheckResourceAttrWith(effective, "counts", func(v string) error {
					if !regexp.MustCompile(`"total":\s*2`).MatchString(v) || !regexp.MustCompile(`"without_row":\s*1`).MatchString(v) {
						return fmt.Errorf("the counts are %s", v)
					}
					return nil
				}),
				tfresource.TestCheckResourceAttr(effective, "items.0.repo", "acme/api"),
				tfresource.TestCheckResourceAttr(effective, "items.0.has_row", "true"),
				tfresource.TestCheckResourceAttrWith(effective, "items.0.settings", func(v string) error {
					if !regexp.MustCompile(`"check_network":\s*\{[^}]*"following":\s*false`).MatchString(v) && !regexp.MustCompile(`"check_network":\{"following":false`).MatchString(v) {
						return fmt.Errorf("check_network is set on the row, so it does not follow: %s", v)
					}
					return nil
				}),
				tfresource.TestCheckResourceAttr(effective, "items.1.has_row", "false"),
			),
		}},
	})
}

func TestEffectiveRepoSettingsStopsAtThePageBound(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.CapPages(1)
	f.Reachable(repoNames(201)...)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config:      effectiveConfig(f.URL, ""),
			ExpectError: regexp.MustCompile(`(?s)more\s+than\s+200\s+pages\s+of\s+500\s+rows.*narrow\s+it`),
		}},
	})
}
