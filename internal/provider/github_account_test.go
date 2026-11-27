package provider

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// The real resource: one connected GitHub account's settings, adopted by its login,
// changed only where the block states a value, and left connected on destroy.

func accountRow(login string, id int, managed bool) map[string]any {
	return map[string]any{
		"id": login, "account": login, "account_id": 4000 + id, "account_type": "Organization",
		"installation_id": 9000 + id, "status": "active", "managed": managed, "holds_context_repo": false,
		"repository_count": 2, "repos_truncated": false, "repos_refreshed_at": nil, "repos": []any{login + "/web", login + "/api"},
		"auto_pr_enabled": true, "auto_revise_on_review": false, "draft_prs": nil, "review_enabled": true,
		"link_comments_enabled": true, "auto_merge_requires_verification": false, "review_severity_floor": "high",
	}
}

func accountWorld(t *testing.T) *fakefacade.Fake {
	t.Helper()
	f := fakefacade.New(t, specs.All())
	f.Seed("github_accounts", accountRow("acme", 1, true))
	f.Seed("github_accounts", accountRow("jane", 2, false))
	return f
}

func accountConfig(url, login, body string) string {
	return providerBlock(url) + fmt.Sprintf("resource \"sreagent_github_account\" \"a\" {\n  account = %q\n%s}\n", login, body)
}

const githubAccount = "sreagent_github_account.a"

func TestGitHubAccountIsAdoptedChangedAndLeftConnected(t *testing.T) {
	f := accountWorld(t)
	cfg := func(body string) string { return accountConfig(f.URL, "jane", body) }
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{
				Config: cfg("  review_severity_floor = \"critical\"\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(githubAccount, "id", "jane"),
					resource.TestCheckResourceAttr(githubAccount, "review_enabled", "true"),
					resource.TestCheckResourceAttr(githubAccount, "managed", "false"),
					resource.TestCheckResourceAttr(githubAccount, "installation_id", "9002"),
					func(*terraform.State) error {
						body := f.LastBody("PUT", "github_accounts")
						if !reflect.DeepEqual(body, map[string]any{"review_severity_floor": "critical"}) {
							return fmt.Errorf("the adopting write sent %v, want only the difference", body)
						}
						if row := f.Row("github_accounts", "jane"); row["review_enabled"] != true || row["auto_pr_enabled"] != true {
							return fmt.Errorf("a setting the block leaves out changed: %v", row)
						}
						if row := f.Row("github_accounts", "acme"); row["review_severity_floor"] != "high" {
							return fmt.Errorf("another account's row changed: %v", row)
						}
						return nil
					},
				),
			},
			{Config: cfg("  review_severity_floor = \"critical\"\n"), ConfigPlanChecks: emptyPlan},
			{Config: cfg("  review_severity_floor = \"critical\"\n  review_enabled = false\n"), Check: resource.TestCheckResourceAttr(githubAccount, "review_enabled", "false")},
			{ResourceName: githubAccount, ImportState: true, ImportStateId: "jane", ImportStateVerify: true, Config: cfg("  review_severity_floor = \"critical\"\n  review_enabled = false\n")},
			{ResourceName: githubAccount, ImportState: true, ImportStateKind: resource.ImportBlockWithResourceIdentity, Config: cfg("  review_severity_floor = \"critical\"\n  review_enabled = false\n")},
			{Config: providerBlock(f.URL), Check: func(*terraform.State) error {
				if f.Row("github_accounts", "jane") == nil || f.Calls("DELETE", "github_accounts") != 0 {
					return fmt.Errorf("the destroy removed the account")
				}
				return nil
			}},
			{
				Config:      accountConfig(f.URL, "ghost", ""),
				ExpectError: regexp.MustCompile(`(?s)no\s+sreagent_github_account\s+for\s+ghost.*no\s+door\s+that\s+creates.*Connect\s+the\s+account\s+on\s+the\s+Integrations\s+page\s+first`),
			},
		},
	})
	if n := f.Calls("POST", "github_accounts"); n != 0 {
		t.Fatalf("the resource POSTed %d times to a platform with no create door", n)
	}
}

// The managed account is the row sreagent_github_settings writes: the plan says so
// when the account a block adopts is that one, and not for another account.
func TestGitHubAccountWarnsWhenItIsTheManagedAccount(t *testing.T) {
	f := accountWorld(t)
	d := newDirect(t, specs.GitHubAccount, f)
	managed := d.adopt(map[string]tftypes.Value{"account": str("acme"), "review_enabled": boolean(false)})
	if managed.Diagnostics.HasError() {
		t.Fatal(managed.Diagnostics)
	}
	if !warned(managed.Diagnostics, "This is the organization's managed account", "sreagent_github_settings") {
		t.Errorf("no warning for the managed account: %s", warningText(managed.Diagnostics))
	}
	other := d.adopt(map[string]tftypes.Value{"account": str("jane"), "review_enabled": boolean(false)})
	if other.Diagnostics.HasError() {
		t.Fatal(other.Diagnostics)
	}
	if warned(other.Diagnostics, "managed account") {
		t.Errorf("a warning about the managed account for another account: %s", warningText(other.Diagnostics))
	}
	if !warned(other.Diagnostics, "adopts the organization's current settings", "terraform import sreagent_github_account.<name> jane") {
		t.Errorf("the plan does not say how to import: %s", warningText(other.Diagnostics))
	}
	if f.Calls("PUT", "github_accounts") != 0 {
		t.Error("a plan wrote")
	}
}

// sreagent_github_settings and sreagent_github_account manage different rows of one
// organization side by side: the managed account and another.
func TestGitHubAccountAndGitHubSettingsManageDifferentAccounts(t *testing.T) {
	f := accountWorld(t)
	cfg := providerBlock(f.URL) +
		"resource \"sreagent_github_settings\" \"managed\" {\n  review_enabled = false\n}\n" +
		"resource \"sreagent_github_account\" \"jane\" {\n  account = \"jane\"\n  draft_prs = true\n}\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: cfg, Check: resource.TestCheckResourceAttr("sreagent_github_account.jane", "draft_prs", "true")},
			{Config: cfg, ConfigPlanChecks: emptyPlan},
		},
	})
	if row := f.Row("github_accounts", "acme"); row["draft_prs"] != nil {
		t.Fatalf("the other account's block changed the managed account: %v", row)
	}
}

func TestGitHubAccountDataSourceListsTheAccounts(t *testing.T) {
	f := accountWorld(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_github_accounts\" \"all\" {}\n",
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.sreagent_github_accounts.all", "items.#", "2"),
				resource.TestCheckResourceAttr("data.sreagent_github_accounts.all", "items.0.account", "acme"),
				resource.TestCheckResourceAttr("data.sreagent_github_accounts.all", "items.0.managed", "true"),
			),
		}},
	})
}
