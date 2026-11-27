package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// The platform names a row by more than one spelling: a child by id or by name, an
// account by its login in any case or its installation number. An imported row holds
// what the import id said; a block that names the same row another way is no new
// row: the plan never replaces it (a replacement would be a no-op destroy and a second
// adoption), and the apply only records the block's spelling and writes nothing.

func notReplaced(addr string) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
		plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
	}}
}

func TestGitHubChildAccessImportedByIdIsNotReplacedByTheChildsNameOrAnotherCase(t *testing.T) {
	f := childAccessWorld(t, "all")
	byName := "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"acme-labs\"\n  account            = \"ACME\"\n  mode               = \"all\"\n}\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{ResourceName: childAccess, ImportState: true, ImportStatePersist: true, ImportStateId: "child-1?account=acme", Config: providerBlock(f.URL) + byName},
			{Config: providerBlock(f.URL) + byName, ConfigPlanChecks: notReplaced(childAccess)},
			{Config: providerBlock(f.URL) + byName, ConfigPlanChecks: emptyPlan},
		},
	})
	if n := f.ChildAccessWrites(); n != 0 {
		t.Fatalf("naming the same row another way wrote %d times", n)
	}
	if n := f.Calls("PUT", "github_child_access"); n != 0 {
		t.Fatalf("naming the same row another way sent %d PUTs", n)
	}
}

// A different child or account is a different row, and does replace.
func TestGitHubChildAccessANamedOtherRowStillReplaces(t *testing.T) {
	f := childAccessWorld(t, "all")
	f.GitHubAccount("jane", "jane/tools")
	f.LinkChild("child-1", "acme-labs", "jane", "all", true)
	cfg := func(account string) string {
		return providerBlock(f.URL) + "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"child-1\"\n  account            = \"" + account + "\"\n  mode               = \"all\"\n}\n"
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: cfg("acme")},
			{Config: cfg("jane"), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
				plancheck.ExpectResourceAction(childAccess, plancheck.ResourceActionDestroyBeforeCreate),
			}}},
		},
	})
}

func TestGitHubAccountImportedInOneCaseIsNotReplacedByAnother(t *testing.T) {
	f := accountWorld(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{ResourceName: githubAccount, ImportState: true, ImportStatePersist: true, ImportStateId: "jane", Config: accountConfig(f.URL, "Jane", "")},
			{Config: accountConfig(f.URL, "Jane", ""), ConfigPlanChecks: notReplaced(githubAccount)},
			{Config: accountConfig(f.URL, "Jane", ""), ConfigPlanChecks: emptyPlan},
		},
	})
	if n := f.Calls("PUT", "github_accounts"); n != 0 {
		t.Fatalf("naming the account in another case sent %d PUTs", n)
	}
}

// The child may be imported by its name, the way the block names it: no difference at all.
func TestGitHubChildAccessImportsByTheChildsNameAndPlansNothing(t *testing.T) {
	f := childAccessWorld(t, "all")
	cfg := providerBlock(f.URL) + "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"acme-labs\"\n  account            = \"acme\"\n  mode               = \"all\"\n}\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{ResourceName: childAccess, ImportState: true, ImportStatePersist: true, ImportStateId: "acme-labs?account=acme", Config: cfg},
			{Config: cfg, ConfigPlanChecks: emptyPlan},
		},
	})
}

// A 404 can mean a missing row or a platform that does not serve the resource at all; the
// error says both, and names the release that serves it.
func TestAdoptOnlyResourcesNameThePlatformReleaseTheyNeedWhenTheRowIsMissing(t *testing.T) {
	f := childAccessWorld(t, "all")
	want := regexp.MustCompile(`(?s)older\s+than\s+v0\.538\.0.*needs\s+platform\s+v0\.538\.0\s+or\s+later`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: accountConfig(f.URL, "ghost", ""), ExpectError: want},
			{Config: providerBlock(f.URL) + "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"nobody\"\n  account            = \"acme\"\n  mode               = \"all\"\n}\n", ExpectError: want},
		},
	})
}
