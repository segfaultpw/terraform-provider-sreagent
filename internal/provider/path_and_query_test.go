package provider

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
)

// scratchChildAccess is the shape of what a parent organization gives one child
// of one GitHub account: a row the platform has no door to create or delete,
// addressed by the child in the path and the account in the query, adopted by the
// block and left in place when the resource goes, written against a preview when
// the change takes something away. It stands for the real spec until one uses
// the capabilities.
var scratchChildAccess = engine.Spec{
	Key: "github_child_access", TypeName: "github_child_access", ListName: "github_child_access_rows",
	Description: "Scratch: what a parent gives one child of one GitHub account.",
	Shape:       engine.PathAndQuery, IDAttr: "child_organization", QueryAttr: "account", Lifecycle: true,
	AdoptOnly: true, AdoptHint: "A child organization is linked under its parent by a platform administrator.",
	Previewed: true, ProbeFingerprint: "preview", AccessPreview: true,
	LeaveOnDestroy: "The child keeps what it was given; change it here or in the app.",
	Attrs: []engine.Attr{
		{Name: "child_organization", Kind: engine.String, Address: true, Required: true, CreateOnly: true, Description: "The child organization."},
		{Name: "account", Kind: engine.String, Address: true, Required: true, CreateOnly: true, Description: "The GitHub account."},
		{Name: "mode", Kind: engine.String, UpdateOnly: true, OneOf: []string{"all", "selected", "none"}, Description: "What the child may use."},
		{Name: "repositories", Kind: engine.StringList, UpdateOnly: true, Unordered: true, Recomputed: true, MaxItems: 500, Description: "The repositories of a selected child."},
		{Name: "context_repo_shared", Kind: engine.Bool, UpdateOnly: true, Description: "Whether the child may use the context repository."},
		{Name: "child_name", Kind: engine.String, Computed: true, Description: "The child's name."},
		{Name: "repository_count", Kind: engine.Int, Computed: true, Description: "How many repositories the child reaches."},
		{Name: "preview_fingerprint", Kind: engine.String, Computed: true, Local: true, Description: "The fingerprint of the preview the plan took."},
	},
}

const caAddr = "sreagent_github_child_access.c"

var caVersions = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)}

func caWorld(t *testing.T, mode string, repos ...string) (*fakefacade.Fake, map[string]func() (tfprotov6.ProviderServer, error)) {
	t.Helper()
	f, fx := scratch(t, scratchChildAccess)
	f.GitHubAccount("acme", "acme/web", "acme/infra", "acme/api")
	f.GitHubAccount("jane", "jane/tools")
	f.LinkChild("child-1", "acme-labs", "acme", mode, true, repos...)
	return f, fx
}

func caConfig(url, body string) string {
	return providerBlock(url) + "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"child-1\"\n  account            = \"acme\"\n" + body + "}\n"
}

func caFingerprints(f *fakefacade.Fake) []string {
	var out []string
	for _, b := range f.PutBodies("github_child_access") {
		fp, _ := b["expected_fingerprint"].(string)
		out = append(out, fp)
	}
	return out
}

// Adopting a row that needs a widening: the plan asks the platform for the
// preview with a fingerprint no preview carries (so nothing is written, though the
// platform writes a widening at once), and the apply writes against the
// fingerprint of that preview.
func TestChildAccessPlanPreviewsAWideningWithoutWritingAndTheApplySendsThatFingerprint(t *testing.T) {
	f, fx := caWorld(t, "none")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{
			{
				Config:   caConfig(f.URL, "  mode = \"all\"\n"),
				PlanOnly: true, ExpectNonEmptyPlan: true,
			},
			{
				PreConfig: func() {
					if f.ChildAccessWrites() != 0 {
						t.Fatal("a plan wrote to the platform")
					}
					if f.ChildAccessPreviews() == 0 {
						t.Fatal("the plan took no preview")
					}
				},
				Config: caConfig(f.URL, "  mode = \"all\"\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(caAddr, "mode", "all"),
					resource.TestCheckResourceAttr(caAddr, "id", "child-1?account=acme"),
					resource.TestCheckResourceAttr(caAddr, "child_organization", "child-1"),
					func(*terraform.State) error {
						if f.ChildAccessWrites() != 1 {
							return fmt.Errorf("want one write, got %d", f.ChildAccessWrites())
						}
						fps := caFingerprints(f)
						last := fps[len(fps)-1]
						if last == "" || last == "preview" {
							return fmt.Errorf("the write carried %q, want the fingerprint of a preview (all: %v)", last, fps)
						}
						for _, fp := range fps[:len(fps)-1] {
							if fp != "preview" {
								return fmt.Errorf("a request before the write carried %q (all: %v)", fp, fps)
							}
						}
						if mode, _, _, _ := f.ChildAccessRow("child-1", "acme"); mode != "all" {
							return fmt.Errorf("the platform holds %q", mode)
						}
						return nil
					},
				),
			},
			{Config: caConfig(f.URL, "  mode = \"all\"\n"), ConfigPlanChecks: emptyPlan},
		},
	})
	if n := f.Calls("POST", "github_child_access") + f.Calls("DELETE", "github_child_access"); n != 0 {
		t.Fatalf("the resource sent %d creates or deletes to a platform that has none", n)
	}
}

// A block that already says what the platform holds adopts it without a write.
func TestChildAccessAdoptsARowThatAlreadyMatchesWithoutAWrite(t *testing.T) {
	f, fx := caWorld(t, "selected", "acme/web")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{
			{
				Config: caConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(caAddr, "repositories.#", "1"),
					resource.TestCheckResourceAttr(caAddr, "child_name", "acme-labs"),
					func(*terraform.State) error {
						if n := len(f.PutBodies("github_child_access")); n != 0 {
							return fmt.Errorf("adopting a matching row sent %d PUTs", n)
						}
						return nil
					},
				),
			},
			{Config: caConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n"), ConfigPlanChecks: emptyPlan},
			{Config: caConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n"), ConfigPlanChecks: emptyPlan},
		},
	})
}

// A narrowing is applied against the preview the plan took, and what the block
// leaves out is left as it is.
func TestChildAccessNarrowsAgainstThePlansPreviewAndLeavesWhatTheBlockOmits(t *testing.T) {
	f, fx := caWorld(t, "all")
	f.ChildHoldsOpenFixPRs("child-1", "acme", 2)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{
			{Config: caConfig(f.URL, "  mode = \"all\"\n")},
			{
				Config: caConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(caAddr, plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(caAddr, "mode", "selected"),
					resource.TestCheckResourceAttr(caAddr, "repository_count", "1"),
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
			{Config: caConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n"), ConfigPlanChecks: emptyPlan},
			// Going back to all leaves the list for the platform to answer empty: the
			// attribute follows the mode it depends on.
			{
				Config: caConfig(f.URL, "  mode = \"all\"\n"),
				Check:  resource.TestCheckResourceAttr(caAddr, "repositories.#", "0"),
			},
			{
				Config: caConfig(f.URL, "  context_repo_shared = false\n"),
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

// What would move changed between the plan and the apply: the platform answers
// 409, nothing is written, and the error says to plan again.
func TestChildAccessMovedSinceThePlanFailsTheApplyWithoutWriting(t *testing.T) {
	f, fx := caWorld(t, "all")
	f.ChildHoldsOpenFixPRs("child-1", "acme", 2)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{
			{Config: caConfig(f.URL, "  mode = \"all\"\n")},
			{
				PreConfig: func() {
					f.ChildChangesOnNextFingerprintedPut(func(_ *string, _ *[]string, _ *bool, open *int) { *open = 9 })
				},
				Config:      caConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n"),
				ExpectError: regexp.MustCompile(`(?s)changed\s+since\s+the\s+plan.*nothing\s+was\s+written.*terraform\s+plan\s+again`),
			},
		},
	})
	if mode, _, _, _ := f.ChildAccessRow("child-1", "acme"); mode != "all" {
		t.Fatalf("a stale write changed the row to %q", mode)
	}
}

// The platform has no door that removes the row, so a destroy writes nothing, and
// says so.
func TestChildAccessDestroyLeavesTheRowInPlace(t *testing.T) {
	f, fx := caWorld(t, "none")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{
			{Config: caConfig(f.URL, "  mode = \"all\"\n")},
			{
				Config: providerBlock(f.URL),
				Check: func(*terraform.State) error {
					if mode, _, _, ok := f.ChildAccessRow("child-1", "acme"); !ok || mode != "all" {
						return fmt.Errorf("the destroy changed the row: %q %v", mode, ok)
					}
					if n := f.Calls("DELETE", "github_child_access"); n != 0 {
						return fmt.Errorf("the destroy sent %d DELETEs", n)
					}
					return nil
				},
			},
		},
	})
}

// Importing takes the child and the account, and the row can then be planned
// like any other.
func TestChildAccessImportsByTheChildAndTheAccount(t *testing.T) {
	f, fx := caWorld(t, "selected", "acme/web")
	cfg := caConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{
			{Config: cfg},
			{ResourceName: caAddr, ImportState: true, ImportStateId: "child-1?account=acme", ImportStateVerify: true, ImportStateVerifyIgnore: []string{"preview_fingerprint"}, Config: cfg},
			{ResourceName: caAddr, ImportState: true, ImportStateId: "child-1", ImportStateVerify: true, ImportStateVerifyIgnore: []string{"preview_fingerprint"}, Config: cfg},
			{ResourceName: caAddr, ImportState: true, ImportStateKind: resource.ImportBlockWithResourceIdentity, Config: cfg},
			{ResourceName: caAddr, ImportState: true, ImportStateId: "child-1?environment=acme", Config: cfg, ExpectError: regexp.MustCompile(`Import the row by its key`)},
		},
	})
}

// With no row on the platform there is nothing to adopt: the plan says where to
// make it, and nothing is written.
func TestChildAccessWithoutARowSaysItHasNoDoorToCreateOne(t *testing.T) {
	f, fx := caWorld(t, "all")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{{
			Config:      providerBlock(f.URL) + "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"ghost\"\n  account = \"acme\"\n  mode = \"all\"\n}\n",
			ExpectError: regexp.MustCompile(`(?s)no\s+sreagent_github_child_access\s+for\s+ghost.*no\s+door\s+that\s+creates.*Connect\s+or\s+link\s+it\s+in\s+the\s+app\s+first`),
		}},
	})
	if n := len(f.PutBodies("github_child_access")); n != 0 {
		t.Fatalf("%d PUTs reached a platform with no row to adopt", n)
	}
}

// The platform refuses a repository that is not the account's, and the plan
// shows its sentence instead of a preview.
func TestChildAccessShowsThePlatformsRefusalAtPlan(t *testing.T) {
	f, fx := caWorld(t, "all")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{{
			Config:      caConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/nope\"]\n"),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`(?s)Could not plan.*acme/nope\s+is\s+not\s+a\s+repository\s+of\s+acme`),
		}},
	})
	if f.ChildAccessWrites() != 0 {
		t.Fatal("a refused plan wrote")
	}
}

// A key that can plan but not apply cannot ask for the preview: the plan says so,
// and the apply is refused before anything is written.
func TestChildAccessReadOnlyKeyPlansButCannotApply(t *testing.T) {
	f, fx := caWorld(t, "none")
	f.ReadOnly()
	cfg := caConfig(f.URL, "  mode = \"all\"\n")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{
			{Config: cfg, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: cfg, ExpectError: regexp.MustCompile(`api:config_read, which can plan but not apply`)},
		},
	})
	if f.ChildAccessWrites() != 0 {
		t.Fatal("a read-only key wrote")
	}
}

// An address known only at apply cannot be previewed by the plan; the apply reads
// the row, previews the change itself and applies that fingerprint.
func TestChildAccessAddressKnownOnlyAtApplyIsPreviewedByTheApply(t *testing.T) {
	f, fx := caWorld(t, "none")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "resource \"terraform_data\" \"child\" {\n  input = \"child-1\"\n}\n" +
				"resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = terraform_data.child.output\n  account = \"acme\"\n  mode = \"all\"\n}\n",
			Check: func(*terraform.State) error {
				if mode, _, _, _ := f.ChildAccessRow("child-1", "acme"); mode != "all" {
					return fmt.Errorf("the platform holds %q", mode)
				}
				fps := caFingerprints(f)
				if len(fps) != 2 || fps[0] != "preview" || fps[1] == "" || fps[1] == "preview" {
					return fmt.Errorf("want the apply's own probe and then the write against its fingerprint, got %v", fps)
				}
				return nil
			},
		}},
	})
}

// Changing the address replaces the resource, which leaves the old row where it
// is and adopts the new one.
func TestChildAccessChangingTheAccountReplacesTheResource(t *testing.T) {
	f, fx := caWorld(t, "all")
	f.LinkChild("child-1", "acme-labs", "jane", "none", true)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{
			{Config: caConfig(f.URL, "  mode = \"all\"\n")},
			{
				Config: providerBlock(f.URL) + "resource \"sreagent_github_child_access\" \"c\" {\n  child_organization = \"child-1\"\n  account = \"jane\"\n  mode = \"all\"\n}\n",
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(caAddr, plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: func(*terraform.State) error {
					if mode, _, _, _ := f.ChildAccessRow("child-1", "acme"); mode != "all" {
						return fmt.Errorf("the old account's row changed to %q", mode)
					}
					if mode, _, _, _ := f.ChildAccessRow("child-1", "jane"); mode != "all" {
						return fmt.Errorf("the new account's row holds %q", mode)
					}
					return nil
				},
			},
		},
	})
}

// The warnings are read by a person; they are checked on the resource itself,
// because Terraform's test harness discards them.

func TestChildAccessAdoptionPlanSaysWhatTheRowHoldsWhatChangesAndHowToImport(t *testing.T) {
	f, _ := caWorld(t, "all")
	f.ChildHoldsOpenFixPRs("child-1", "acme", 2)
	d := newDirect(t, scratchChildAccess, f)
	resp := d.adopt(map[string]tftypes.Value{
		"child_organization": str("child-1"), "account": str("acme"),
		"mode": str("selected"), "repositories": strList("acme/web"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	for _, parts := range [][]string{
		{"What this change moves", "acme-labs", "the GitHub account acme", "every repository of the account now and will use only acme/web", "stops reaching 2 repositories: acme/api, acme/infra", "4 open fix pull requests of the child's stay open on GitHub", "with one comment saying SRE Agent no longer manages it", "Nothing is closed or deleted"},
		{"adopts the organization's current settings", "mode from \"all\" to \"selected\"", "terraform import sreagent_github_child_access.<name> child-1?account=acme"},
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
}

func TestChildAccessDestroyPlanAndApplyWarnWhatStaysAndWriteNothing(t *testing.T) {
	f, _ := caWorld(t, "all")
	d := newDirect(t, scratchChildAccess, f)
	state := d.value(map[string]tftypes.Value{"id": str("child-1?account=acme"), "child_organization": str("child-1"), "account": str("acme"), "mode": str("all")})
	resp := d.plan(state, d.none())
	if !warned(resp.Diagnostics, "What destroying this resource leaves", "cannot be deleted", "The child keeps what it was given") {
		t.Errorf("the destroy plan says nothing of what stays: %s", warningText(resp.Diagnostics))
	}
	del := d.destroy(state)
	if del.Diagnostics.HasError() || !warned(del.Diagnostics, "Left in place", "The child keeps what it was given") {
		t.Errorf("the destroy must warn and not fail: %v", del.Diagnostics)
	}
	if f.Calls("DELETE", "github_child_access") != 0 || f.ChildAccessWrites() != 0 {
		t.Error("a destroy reached the platform")
	}
}

// scratchAccount is a natural-key row nothing can create or delete: the shape of
// one connected GitHub account's settings. It is adopted by the block and left in
// place when the resource goes.
var scratchAccount = engine.Spec{
	Key: "scratch_account", TypeName: "scratch_account", ListName: "scratch_accounts",
	Description: "Scratch: one connected account's settings.",
	Shape:       engine.NaturalKey, IDAttr: "account", Lifecycle: true,
	AdoptOnly: true, AdoptHint: "Connect the account on the Integrations page.",
	LeaveOnDestroy: "The account stays connected; disconnect it on the Integrations page.",
	Attrs: []engine.Attr{
		{Name: "account", Kind: engine.String, Address: true, Required: true, CreateOnly: true, Description: "The account."},
		{Name: "review_enabled", Kind: engine.Bool, UpdateOnly: true, Description: "Review pull requests."},
		{Name: "review_severity_floor", Kind: engine.String, UpdateOnly: true, OneOf: []string{"critical", "high", "medium", "low"}, Description: "The floor."},
		{Name: "managed", Kind: engine.Bool, Computed: true, Description: "Whether it is the managed account."},
	},
}

func TestAnAdoptedNaturalKeyRowIsAdoptedChangedAndLeftInPlace(t *testing.T) {
	f, fx := scratch(t, scratchAccount)
	f.Seed("scratch_account", map[string]any{"id": "acme", "account": "acme", "review_enabled": true, "review_severity_floor": "high", "managed": true})
	cfg := func(body string) string {
		return providerBlock(f.URL) + "resource \"sreagent_scratch_account\" \"a\" {\n  account = \"acme\"\n" + body + "}\n"
	}
	const addr = "sreagent_scratch_account.a"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fx,
		TerraformVersionChecks:   caVersions,
		Steps: []resource.TestStep{
			{
				Config: cfg("  review_severity_floor = \"critical\"\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "acme"),
					resource.TestCheckResourceAttr(addr, "review_enabled", "true"),
					resource.TestCheckResourceAttr(addr, "managed", "true"),
					func(*terraform.State) error {
						body := f.LastBody("PUT", "scratch_account")
						if !reflect.DeepEqual(body, map[string]any{"review_severity_floor": "critical"}) {
							return fmt.Errorf("the adopting write sent %v, want only the difference", body)
						}
						if row := f.Row("scratch_account", "acme"); row["review_enabled"] != true {
							return fmt.Errorf("a setting the block leaves out changed: %v", row)
						}
						return nil
					},
				),
			},
			{Config: cfg("  review_severity_floor = \"critical\"\n"), ConfigPlanChecks: emptyPlan},
			{Config: cfg("  review_severity_floor = \"critical\"\n  review_enabled = false\n"), Check: resource.TestCheckResourceAttr(addr, "review_enabled", "false")},
			{ResourceName: addr, ImportState: true, ImportStateId: "acme", ImportStateVerify: true, Config: cfg("  review_severity_floor = \"critical\"\n  review_enabled = false\n")},
			{Config: providerBlock(f.URL), Check: func(*terraform.State) error {
				if f.Row("scratch_account", "acme") == nil || f.Calls("DELETE", "scratch_account") != 0 {
					return fmt.Errorf("the destroy removed the row")
				}
				return nil
			}},
			{
				Config:      providerBlock(f.URL) + "resource \"sreagent_scratch_account\" \"a\" {\n  account = \"ghost\"\n}\n",
				ExpectError: regexp.MustCompile(`(?s)no\s+sreagent_scratch_account\s+for\s+ghost.*Connect\s+the\s+account\s+on\s+the\s+Integrations\s+page`),
			},
		},
	})
	if n := f.Calls("POST", "scratch_account"); n != 0 {
		t.Fatalf("the resource POSTed %d times to a platform with no create door", n)
	}
}
