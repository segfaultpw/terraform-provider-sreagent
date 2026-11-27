package provider

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// The real resource, against the fake facade of the platform's child access door:
// adopted, written against a preview when it takes repositories away, written at
// once when it does not, and left in place on destroy.

const childAccess = "sreagent_github_child_access.c"

var childAccessVersions = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)}

// childAccessWorld is a parent that connected acme (three repositories) and jane
// (one), with one child linked under it holding mode of acme.
func childAccessWorld(t *testing.T, mode string, repos ...string) *fakefacade.Fake {
	t.Helper()
	f := fakefacade.New(t, specs.All())
	f.GitHubAccount("acme", "acme/web", "acme/infra", "acme/api")
	f.GitHubAccount("jane", "jane/tools")
	f.LinkChild("child-1", "acme-labs", "acme", mode, true, repos...)
	return f
}

func childAccessConfig(url, body string) string {
	return providerBlock(url) + "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"child-1\"\n  account            = \"acme\"\n" + body + "}\n"
}

func putFingerprints(f *fakefacade.Fake) []string {
	var out []string
	for _, b := range f.PutBodies("github_child_access") {
		fp, _ := b["expected_fingerprint"].(string)
		out = append(out, fp)
	}
	return out
}

// A change that takes nothing away is written at once, so the plan must ask for
// its preview with a fingerprint no preview carries (and write nothing), and the
// apply sends the fingerprint of the preview it was shown.
func TestGitHubChildAccessWidensAtOnceAndAdoptsTheRow(t *testing.T) {
	f := childAccessWorld(t, "none")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: childAccessConfig(f.URL, "  mode = \"all\"\n"), PlanOnly: true, ExpectNonEmptyPlan: true},
			{
				PreConfig: func() {
					if f.ChildAccessWrites() != 0 {
						t.Fatal("a plan wrote to the platform")
					}
				},
				Config: childAccessConfig(f.URL, "  mode = \"all\"\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(childAccess, "id", "child-1?account=acme"),
					resource.TestCheckResourceAttr(childAccess, "mode", "all"),
					resource.TestCheckResourceAttr(childAccess, "child_name", "acme-labs"),
					resource.TestCheckResourceAttr(childAccess, "repository_count", "3"),
					func(*terraform.State) error {
						if f.ChildAccessWrites() != 1 {
							return fmt.Errorf("want one write, got %d", f.ChildAccessWrites())
						}
						fps := putFingerprints(f)
						if last := fps[len(fps)-1]; last == "" || last == "preview" {
							return fmt.Errorf("the write carried %q, want the fingerprint of a preview (%v)", last, fps)
						}
						return nil
					},
				),
			},
			{Config: childAccessConfig(f.URL, "  mode = \"all\"\n"), ConfigPlanChecks: emptyPlan},
		},
	})
	if n := f.Calls("POST", "github_child_access") + f.Calls("DELETE", "github_child_access"); n != 0 {
		t.Fatalf("the resource sent %d creates or deletes to a platform with no such door", n)
	}
}

// A narrowing is written against the preview the plan took, a setting the block
// leaves out is left as it is, and going back to all leaves the list for the
// platform to answer empty.
func TestGitHubChildAccessNarrowsAgainstThePlansPreview(t *testing.T) {
	f := childAccessWorld(t, "all")
	f.ChildHoldsOpenFixPRs("child-1", "acme", 2)
	selected := "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: childAccessConfig(f.URL, "  mode = \"all\"\n")},
			{
				Config: childAccessConfig(f.URL, selected),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(childAccess, plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(childAccess, "mode", "selected"),
					resource.TestCheckResourceAttr(childAccess, "repository_count", "1"),
					func(*terraform.State) error {
						mode, repos, shared, _ := f.ChildAccessRow("child-1", "acme")
						if mode != "selected" || !reflect.DeepEqual(repos, []string{"acme/web"}) || !shared {
							return fmt.Errorf("the platform holds %s %v shared %v", mode, repos, shared)
						}
						last := f.LastBody("PUT", "github_child_access")
						if _, sent := last["context_repo_shared"]; sent {
							return fmt.Errorf("the write named a setting the block never stated: %v", last)
						}
						if fp, _ := last["expected_fingerprint"].(string); fp == "" || fp == "preview" {
							return fmt.Errorf("the write carried %q", fp)
						}
						return nil
					},
				),
			},
			{Config: childAccessConfig(f.URL, selected), ConfigPlanChecks: emptyPlan},
			{Config: childAccessConfig(f.URL, "  mode = \"all\"\n"), Check: resource.TestCheckResourceAttr(childAccess, "repositories.#", "0")},
			{
				Config: childAccessConfig(f.URL, "  context_repo_shared = false\n"),
				Check: func(*terraform.State) error {
					if _, _, shared, _ := f.ChildAccessRow("child-1", "acme"); shared {
						return fmt.Errorf("sharing was not switched off")
					}
					return nil
				},
			},
		},
	})
}

// What would move changed between the plan and the apply: the platform answers 409,
// nothing is written, and the error says to plan again.
func TestGitHubChildAccessMovedSinceThePlanFailsWithoutWriting(t *testing.T) {
	f := childAccessWorld(t, "all")
	f.ChildHoldsOpenFixPRs("child-1", "acme", 2)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: childAccessConfig(f.URL, "  mode = \"all\"\n")},
			{
				PreConfig: func() {
					f.ChildChangesOnNextFingerprintedPut(func(_ *string, _ *[]string, _ *bool, open *int) { *open = 9 })
				},
				Config:      childAccessConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n"),
				ExpectError: regexp.MustCompile(`(?s)changed\s+since\s+the\s+plan.*nothing\s+was\s+written.*terraform\s+plan\s+again`),
			},
		},
	})
	if mode, _, _, _ := f.ChildAccessRow("child-1", "acme"); mode != "all" {
		t.Fatalf("a stale write changed the row to %q", mode)
	}
}

// The plan says what a narrowing moves, in the platform's own terms, and nothing is
// written; the destroy leaves the row and says so.
func TestGitHubChildAccessPlanNamesWhatMovesAndDestroyLeavesTheRow(t *testing.T) {
	f := childAccessWorld(t, "all")
	f.ChildHoldsOpenFixPRs("child-1", "acme", 2)
	d := newDirect(t, specs.GitHubChildAccess, f)
	resp := d.adopt(map[string]tftypes.Value{
		"child_organization": str("child-1"), "account": str("acme"),
		"mode": str("selected"), "repositories": strList("acme/web"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	for _, parts := range [][]string{
		{"What this change moves", "acme-labs", "stops reaching 2 repositories: acme/api, acme/infra", "stay open on GitHub", "Nothing is closed or deleted"},
		{"terraform import sreagent_github_child_access.<name> child-1?account=acme"},
	} {
		if !warned(resp.Diagnostics, parts...) {
			t.Errorf("no warning holds %q: %s", parts, warningText(resp.Diagnostics))
		}
	}
	var fp types.String
	resp.Plan.GetAttribute(context.Background(), path.Root("preview_fingerprint"), &fp)
	if fp.IsNull() || fp.IsUnknown() || fp.ValueString() == "" {
		t.Errorf("the plan carries no fingerprint to the apply: %v", fp)
	}
	if f.ChildAccessWrites() != 0 {
		t.Error("a plan wrote")
	}

	state := d.value(map[string]tftypes.Value{"id": str("child-1?account=acme"), "child_organization": str("child-1"), "account": str("acme"), "mode": str("all")})
	del := d.destroy(state)
	if del.Diagnostics.HasError() || !warned(del.Diagnostics, "Left in place", "The child keeps what it was given") {
		t.Errorf("the destroy must warn and not fail: %v", del.Diagnostics)
	}
	if f.Calls("DELETE", "github_child_access") != 0 || f.ChildAccessWrites() != 0 {
		t.Error("a destroy reached the platform")
	}
}

// A destroy through Terraform's own lifecycle changes nothing on the platform.
func TestGitHubChildAccessDestroyLeavesWhatTheChildWasGiven(t *testing.T) {
	f := childAccessWorld(t, "none")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: childAccessConfig(f.URL, "  mode = \"all\"\n")},
			{Config: providerBlock(f.URL), Check: func(*terraform.State) error {
				if mode, _, _, ok := f.ChildAccessRow("child-1", "acme"); !ok || mode != "all" {
					return fmt.Errorf("the destroy changed the row: %q %v", mode, ok)
				}
				if n := f.Calls("DELETE", "github_child_access"); n != 0 {
					return fmt.Errorf("the destroy sent %d DELETEs", n)
				}
				return nil
			}},
		},
	})
}

// Importing takes the child and the account, or the child alone while one account
// is connected, and the row can then be planned like any other.
func TestGitHubChildAccessImportsByTheChildAndTheAccount(t *testing.T) {
	f := childAccessWorld(t, "selected", "acme/web")
	cfg := childAccessConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: cfg},
			{ResourceName: childAccess, ImportState: true, ImportStateId: "child-1?account=acme", ImportStateVerify: true, ImportStateVerifyIgnore: []string{"preview_fingerprint"}, Config: cfg},
			{ResourceName: childAccess, ImportState: true, ImportStateKind: resource.ImportBlockWithResourceIdentity, Config: cfg},
			{ResourceName: childAccess, ImportState: true, ImportStateId: "child-1?environment=acme", Config: cfg, ExpectError: regexp.MustCompile(`Import the row by its key`)},
		},
	})
}

// A child or an account that does not exist has nothing to adopt: the plan says
// where to make it and nothing is written.
func TestGitHubChildAccessWithoutARowSaysWhereToLinkOrConnectIt(t *testing.T) {
	f := childAccessWorld(t, "all")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{
				Config:      providerBlock(f.URL) + "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"ghost\"\n  account = \"acme\"\n  mode = \"all\"\n}\n",
				ExpectError: regexp.MustCompile(`(?s)no\s+sreagent_github_child_access\s+for\s+ghost.*no\s+door\s+that\s+creates.*Link\s+the\s+child\s+organization.*connect\s+the\s+GitHub\s+account\s+on\s+the\s+Integrations\s+page`),
			},
			{
				Config:      providerBlock(f.URL) + "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"child-1\"\n  account = \"nobody\"\n  mode = \"all\"\n}\n",
				ExpectError: regexp.MustCompile(`(?s)no\s+sreagent_github_child_access\s+for\s+child-1.*no\s+door\s+that\s+creates`),
			},
		},
	})
	if n := len(f.PutBodies("github_child_access")); n != 0 {
		t.Fatalf("%d PUTs reached a platform with no row to adopt", n)
	}
}

// The platform takes a list of repositories only with selected, and a selected child
// needs one: both are plan errors, and nothing reaches the platform.
func TestGitHubChildAccessRefusesRepositoriesWithTheWrongMode(t *testing.T) {
	f := childAccessWorld(t, "all")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{
				Config:      childAccessConfig(f.URL, "  mode = \"all\"\n  repositories = [\"acme/web\"]\n"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Repositories\s+need\s+mode\s+selected`),
			},
			{
				Config:      childAccessConfig(f.URL, "  mode = \"selected\"\n  repositories = []\n"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+the\s+repositories\s+to\s+give`),
			},
			{
				Config:      childAccessConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/nope\"]\n"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)acme/nope\s+is\s+not\s+a\s+repository\s+of\s+acme`),
			},
		},
	})
	if f.ChildAccessWrites() != 0 {
		t.Fatal("a refused plan wrote")
	}
}

// A key that can plan but not apply cannot ask for the preview: the plan says so and
// the apply is refused before anything is written.
func TestGitHubChildAccessReadOnlyKeyPlansButCannotApply(t *testing.T) {
	f := childAccessWorld(t, "none")
	f.ReadOnly()
	cfg := childAccessConfig(f.URL, "  mode = \"all\"\n")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: cfg, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: cfg, ExpectError: regexp.MustCompile(`api:config_read, which can plan but not apply`)},
		},
	})
	if f.ChildAccessWrites() != 0 {
		t.Fatal("a read-only key wrote")
	}
}

// The list data source answers every row of the parent: one per child and account.
func TestGitHubChildAccessDataSourceListsTheRows(t *testing.T) {
	f := childAccessWorld(t, "selected", "acme/web")
	f.LinkChild("child-1", "acme-labs", "jane", "none", false)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_github_child_access\" \"all\" {}\n",
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.sreagent_github_child_access.all", "items.#", "2"),
				resource.TestCheckResourceAttr("data.sreagent_github_child_access.all", "items.0.child_name", "acme-labs"),
			),
		}},
	})
}
