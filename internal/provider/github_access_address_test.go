package provider

import (
	"maps"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// The platform names a row by more than one spelling: a child by id or by name, an
// account by its login in any case or its installation number. The provider takes a
// configured address for the same row only when it equals the halves of the row's id as
// the platform answered them: the child's id exactly, the account without regard to
// case. A name is never proof, because organization names are not unique, so naming the
// child another way (by its name where the import used its id, or the name in another
// case) plans a replacement. That is safe: destroy writes nothing, and the create adopts
// the row it names and writes only what the block states.

func notReplaced(addr string) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
		plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
	}}
}

func replacedChildAccess() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
		plancheck.ExpectResourceAction(childAccess, plancheck.ResourceActionDestroyBeforeCreate),
	}}
}

func childAccessBlock(child, account, body string) string {
	return "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"" + child + "\"\n  account            = \"" + account + "\"\n" + body + "}\n"
}

// A child imported by its id and then named by its name is a replacement that
// changes nothing on the platform: the old row is left in place, the same row is
// adopted again, and an unchanged grant sends nothing.
func TestGitHubChildAccessImportedByIdThenNamedByNameReplacesAndWritesNothing(t *testing.T) {
	f := childAccessWorld(t, "all")
	byName := childAccessBlock("acme-labs", "acme", "  mode = \"all\"\n")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{ResourceName: childAccess, ImportState: true, ImportStatePersist: true, ImportStateId: "child-1?account=acme", Config: providerBlock(f.URL) + byName},
			{Config: providerBlock(f.URL) + byName, ConfigPlanChecks: replacedChildAccess()},
			{Config: providerBlock(f.URL) + byName, ConfigPlanChecks: emptyPlan},
		},
	})
	if n := f.Calls("PUT", "github_child_access"); n != 0 {
		t.Fatalf("naming the same child another way sent %d PUTs", n)
	}
}

// The adoption writes only what the block states and the platform differs on, and
// only to the row it names: the old row was never written.
func TestGitHubChildAccessAReplacementSendsOnlyTheStatedDifference(t *testing.T) {
	f := childAccessWorld(t, "none")
	f.LinkChild("child-2", "other-labs", "acme", "none", true)
	byName := childAccessBlock("acme-labs", "acme", "  mode = \"all\"\n")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{ResourceName: childAccess, ImportState: true, ImportStatePersist: true, ImportStateId: "child-1?account=acme", Config: providerBlock(f.URL) + byName},
			{Config: providerBlock(f.URL) + byName, ConfigPlanChecks: replacedChildAccess()},
			{Config: providerBlock(f.URL) + byName, ConfigPlanChecks: emptyPlan},
		},
	})
	// The plan's probes carry no real fingerprint and the platform writes nothing for
	// them; the one write is the adoption.
	if n := f.ChildAccessWrites(); n != 1 {
		t.Fatalf("%d writes, want the one adopting write", n)
	}
	bodies := f.PutBodies("github_child_access")
	last := maps.Clone(bodies[len(bodies)-1])
	delete(last, "expected_fingerprint")
	if !reflect.DeepEqual(last, map[string]any{"mode": "all"}) {
		t.Fatalf("the adopting write sent %v, want only the stated difference", bodies[len(bodies)-1])
	}
	if mode, _, _, _ := f.ChildAccessRow("child-2", "acme"); mode != "none" {
		t.Fatalf("another child was written: %s", mode)
	}
}

// Two siblings whose names differ only by case: the name is not the row. A block
// edited from the first child's id to the name the second one holds plans a
// replacement, and the platform answers ambiguous for a name two children share:
// the error is surfaced as is and neither child is written.
func TestGitHubChildAccessANameTwoSiblingsShareIsNeverTakenForTheManagedChild(t *testing.T) {
	f := childAccessWorld(t, "all")
	f.LinkChild("child-2", "ACME-LABS", "acme", "all", true)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: providerBlock(f.URL) + childAccessBlock("child-1", "acme", "  mode = \"all\"\n")},
			{
				Config:           providerBlock(f.URL) + childAccessBlock("ACME-LABS", "acme", "  mode = \"none\"\n"),
				ConfigPlanChecks: replacedChildAccess(),
				ExpectError:      regexp.MustCompile(`(?s)Could\s+not\s+create.*More\s+than\s+one\s+child\s+organization\s+has\s+that\s+name\.\s+Use\s+its\s+id`),
			},
		},
	})
	if n := f.ChildAccessWrites(); n != 0 {
		t.Fatalf("a name two children share wrote %d times", n)
	}
	for _, c := range []string{"child-1", "child-2"} {
		if mode, _, _, _ := f.ChildAccessRow(c, "acme"); mode != "all" {
			t.Fatalf("%s was written: %s", c, mode)
		}
	}
}

// Naming the other sibling by its id adopts that child and writes nothing to the first.
func TestGitHubChildAccessTheOtherSiblingByIdIsAdoptedAndTheFirstNeverWritten(t *testing.T) {
	f := childAccessWorld(t, "all")
	f.LinkChild("child-2", "ACME-LABS", "acme", "all", true)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: providerBlock(f.URL) + childAccessBlock("child-1", "acme", "  mode = \"all\"\n")},
			{Config: providerBlock(f.URL) + childAccessBlock("child-2", "acme", "  mode = \"none\"\n"), ConfigPlanChecks: replacedChildAccess()},
			{Config: providerBlock(f.URL) + childAccessBlock("child-2", "acme", "  mode = \"none\"\n"), ConfigPlanChecks: emptyPlan},
		},
	})
	if mode, _, _, _ := f.ChildAccessRow("child-1", "acme"); mode != "all" {
		t.Fatalf("the first sibling was written: %s", mode)
	}
	if mode, _, _, _ := f.ChildAccessRow("child-2", "acme"); mode != "none" {
		t.Fatalf("the adopted sibling holds %s", mode)
	}
	if n := f.ChildAccessWrites(); n != 1 {
		t.Fatalf("%d writes, want one for the adopted child", n)
	}
}

// The account's login is unique in any case, so another case is the same row: one
// in-place update that sends nothing.
func TestGitHubChildAccessTheAccountInAnotherCaseIsTheSameRow(t *testing.T) {
	f := childAccessWorld(t, "all")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{ResourceName: childAccess, ImportState: true, ImportStatePersist: true, ImportStateId: "child-1?account=acme", Config: providerBlock(f.URL) + childAccessBlock("child-1", "ACME", "  mode = \"all\"\n")},
			{Config: providerBlock(f.URL) + childAccessBlock("child-1", "ACME", "  mode = \"all\"\n"), ConfigPlanChecks: notReplaced(childAccess)},
			{Config: providerBlock(f.URL) + childAccessBlock("child-1", "ACME", "  mode = \"all\"\n"), ConfigPlanChecks: emptyPlan},
		},
	})
	if n := f.Calls("PUT", "github_child_access"); n != 0 {
		t.Fatalf("naming the account in another case sent %d PUTs", n)
	}
}

// The installation number for the login is another spelling of the same account, but
// not a proof the provider can make from the id, so it replaces and writes nothing.
func TestGitHubChildAccessTheAccountByNumberReplacesAndWritesNothing(t *testing.T) {
	f := childAccessWorld(t, "all")
	byNumber := childAccessBlock("child-1", "1004", "  mode = \"all\"\n")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{ResourceName: childAccess, ImportState: true, ImportStatePersist: true, ImportStateId: "child-1?account=acme", Config: providerBlock(f.URL) + byNumber},
			{Config: providerBlock(f.URL) + byNumber, ConfigPlanChecks: replacedChildAccess()},
			{Config: providerBlock(f.URL) + byNumber, ConfigPlanChecks: emptyPlan},
		},
	})
	if n := f.Calls("PUT", "github_child_access"); n != 0 {
		t.Fatalf("naming the account by number sent %d PUTs", n)
	}
}

// A different account is a different row, and does replace.
func TestGitHubChildAccessANamedOtherAccountStillReplaces(t *testing.T) {
	f := childAccessWorld(t, "all")
	f.LinkChild("child-1", "acme-labs", "jane", "all", true)
	cfg := func(account string) string {
		return providerBlock(f.URL) + childAccessBlock("child-1", account, "  mode = \"all\"\n")
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: cfg("acme")},
			{Config: cfg("jane"), ConfigPlanChecks: replacedChildAccess()},
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
