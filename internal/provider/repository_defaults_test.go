package provider

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const repoDefaults = "sreagent_repository_defaults.d"

func defaultsConfig(url, body string) string {
	return providerBlock(url) + fmt.Sprintf("resource \"sreagent_repository_defaults\" \"d\" {\n%s\n}\n", body)
}

// bodies are the PUT bodies the defaults received, in order.
func defaultsPuts(f *fakefacade.Fake) []map[string]any { return f.PutBodies("repository_defaults") }

func fingerprintOf(body map[string]any) string {
	s, _ := body["expected_fingerprint"].(string)
	return s
}

func withoutFingerprint(body map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range body {
		if k != "expected_fingerprint" {
			out[k] = v
		}
	}
	return out
}

func seedRepo(t *testing.T, f *fakefacade.Fake, repo string) {
	t.Helper()
	seedOne(t, f.URL, "repo_settings", map[string]any{"repo": repo})
}

// The plan takes the platform's preview and the apply writes against that
// fingerprint. Terraform plans the change again when it applies a saved plan, so
// the preview is taken twice (once at the plan, once at the re-plan) and the
// write once, and the write carries the fingerprint both previews gave.
func TestRepositoryDefaultsPlanPreviewsAndTheApplySendsThatFingerprint(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	seedRepo(t, f, "acme/web")
	seedRepo(t, f, "acme/api")
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{
			{
				Config: defaultsConfig(f.URL, "  auto_dispatch = false"),
				Check: tfresource.ComposeTestCheckFunc(
					tfresource.TestCheckResourceAttr(repoDefaults, "auto_dispatch", "false"),
					tfresource.TestCheckResourceAttr(repoDefaults, "source", "own"),
					func(*terraform.State) error {
						puts := defaultsPuts(f)
						if f.Previews() != 2 || len(puts) != 3 {
							return fmt.Errorf("want two previews and one write, got %d previews and %d PUTs: %v", f.Previews(), len(puts), puts)
						}
						for _, p := range puts[:2] {
							if fingerprintOf(p) != "" {
								return fmt.Errorf("a preview carried a fingerprint: %v", p)
							}
						}
						write := puts[2]
						if fingerprintOf(write) == "" || fingerprintOf(write) != f.LastFingerprint() {
							return fmt.Errorf("the write carried %q, the preview gave %q", fingerprintOf(write), f.LastFingerprint())
						}
						if want := map[string]any{"auto_dispatch": false}; !reflect.DeepEqual(withoutFingerprint(write), want) {
							return fmt.Errorf("the write was %v, want %v", withoutFingerprint(write), want)
						}
						return nil
					},
				),
			},
			{
				Config:           defaultsConfig(f.URL, "  auto_dispatch = false"),
				ConfigPlanChecks: emptyPlan,
				Check: func(*terraform.State) error {
					if n := len(defaultsPuts(f)); n != 3 {
						return fmt.Errorf("a clean plan sent %d PUTs in all, want the 3 of the first step", n)
					}
					return nil
				},
			},
			{ResourceName: repoDefaults, ImportState: true, ImportStateId: "repository_defaults", ImportStateVerify: true, ImportStateVerifyIgnore: []string{"preview_fingerprint"}, Config: defaultsConfig(f.URL, "  auto_dispatch = false")},
		},
	})
}

// A change that happens between the preview and the write fails the apply: the
// platform answers 409, nothing is written, and the error says to plan again.
func TestRepositoryDefaultsMovedSinceThePlanFailsTheApply(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	seedRepo(t, f, "acme/web")
	f.RepositoryAppearsOnNextFingerprintedPut("acme/late")
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config:      defaultsConfig(f.URL, "  auto_dispatch = false"),
			ExpectError: regexp.MustCompile(`(?s)changed\s+since\s+the\s+plan.*nothing\s+was\s+written.*terraform\s+plan\s+again`),
		}},
	})
	if got := f.Defaults()["auto_dispatch"]; got != true {
		t.Fatalf("a refused apply still wrote auto_dispatch = %v", got)
	}
}

// With a key that can plan but not apply, the plan succeeds without a preview.
func TestRepositoryDefaultsReadOnlyKeyPlansWithAWarning(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.ReadOnly()
	cfg := defaultsConfig(f.URL, "  auto_dispatch = false")
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{
			{Config: cfg, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: cfg, ExpectError: regexp.MustCompile(`api:config_read, which can plan but not apply`)},
		},
	})
	if f.Previews() != 0 {
		t.Fatalf("a read-only key was given %d previews", f.Previews())
	}
}

// When the plan could not preview (a plan-only key), the apply previews, judges
// the preview as a plan would and applies the fingerprint it took.
func TestRepositoryDefaultsApplyPreviewsWhenThePlanCouldNot(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	seedRepo(t, f, "acme/web")
	f.RefusePreviews(2)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config: defaultsConfig(f.URL, "  auto_dispatch = false"),
			Check: func(*terraform.State) error {
				puts := defaultsPuts(f)
				if len(puts) != 4 {
					return fmt.Errorf("want two refused previews, one preview and one write, got %v", puts)
				}
				if fingerprintOf(puts[3]) == "" || fingerprintOf(puts[3]) != f.LastFingerprint() {
					return fmt.Errorf("the write carried %q, the apply's preview gave %q", fingerprintOf(puts[3]), f.LastFingerprint())
				}
				if got := f.Defaults()["auto_dispatch"]; got != false {
					return fmt.Errorf("auto_dispatch is %v", got)
				}
				return nil
			},
		}},
	})
}

// Even without a preview at plan time, the apply refuses a change the preview
// says needs a confirmation the configuration lacks.
func TestRepositoryDefaultsApplyWithoutAPlanPreviewStillAsksForTheConfirmation(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.RefusePreviews(2)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config:      defaultsConfig(f.URL, "  check_network = true"),
			ExpectError: regexp.MustCompile(`(?s)Confirmation\s+needed.*network\s+access.*confirm\s*=\s*\["check_network"\]`),
		}},
	})
	if got := f.Defaults()["check_network"]; got != false {
		t.Fatalf("check_network is %v after a refused apply", got)
	}
}

func TestTurningCIOnWithoutConfirmFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config:      defaultsConfig(f.URL, "  allow_ci_config = true"),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`(?s)Confirmation\s+needed.*CI\s+configuration.*needs\s+confirmation:\s+pass\s+confirm\s+ci_config.*confirm\s*=\s*\["ci_config"\]`),
		}},
	})
	if n := len(defaultsPuts(f)); n != 1 {
		t.Fatalf("the failing plan sent %d PUTs, want its one preview", n)
	}
}

func TestTurningCIOnWithConfirmApplies(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := defaultsConfig(f.URL, "  allow_ci_config = true\n  confirm = [\"ci_config\"]")
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{
			{
				Config: cfg,
				Check: tfresource.ComposeTestCheckFunc(
					tfresource.TestCheckResourceAttr(repoDefaults, "allow_ci_config", "true"),
					tfresource.TestCheckNoResourceAttr(repoDefaults, "confirm"),
					func(*terraform.State) error {
						puts := defaultsPuts(f)
						write := puts[len(puts)-1]
						if !reflect.DeepEqual(write["confirm"], []any{"ci_config"}) || write["allow_ci_config"] != true {
							return fmt.Errorf("the write was %v", write)
						}
						return nil
					},
				),
			},
			{Config: cfg, ConfigPlanChecks: emptyPlan},
		},
	})
}

// A confirmation names a change, it never makes one: a configuration that only
// adds or drops it plans nothing and sends nothing.
func TestConfirmAloneNeverWrites(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{
			{Config: defaultsConfig(f.URL, "  auto_dispatch = false")},
			{
				Config:           defaultsConfig(f.URL, "  auto_dispatch = false\n  confirm = [\"ci_config\", \"check_network\"]"),
				ConfigPlanChecks: emptyPlan,
				Check: func(*terraform.State) error {
					if n := len(defaultsPuts(f)); n != 3 {
						return fmt.Errorf("naming a confirmation sent PUTs: %d in all, want the 3 of the first step", n)
					}
					return nil
				},
			},
		},
	})
}

func TestAPreviewRefusalFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.RefuseDefaults("Your own runner needs a fix runner registered first.")
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config:      defaultsConfig(f.URL, "  fix_runner = \"self_hosted\""),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`(?s)The\s+platform\s+refuses\s+this\s+change.*needs\s+a\s+fix\s+runner\s+registered\s+first`),
		}},
	})
}

func TestAStaleIfMatchAtPlanFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{
			{Config: defaultsConfig(f.URL, "  auto_dispatch = false")},
			{
				PreConfig:   func() { f.ChangeDefaultsOnNextPut(map[string]any{"platform_fallback": false}) },
				Config:      defaultsConfig(f.URL, "  auto_dispatch = true"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)changed\s+outside\s+Terraform.*Nothing\s+was\s+written`),
			},
		},
	})
}

// A child organization that follows its parent reads 404. Creating the resource
// gives it its own defaults for all six settings and stops it following; the
// write carries no If-Match, because the row is not the child's.
func TestRepositoryDefaultsInAFollowingChildWarnsItLeavesTheParent(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.DefaultsSource("parent", "Acme Parent", map[string]any{"platform_fallback": false})
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config: defaultsConfig(f.URL, "  auto_dispatch = false"),
			Check: tfresource.ComposeTestCheckFunc(
				tfresource.TestCheckResourceAttr(repoDefaults, "source", "own"),
				tfresource.TestCheckResourceAttr(repoDefaults, "parent_name", "Acme Parent"),
				tfresource.TestCheckResourceAttr(repoDefaults, "platform_fallback", "false"),
				func(*terraform.State) error {
					for _, im := range f.IfMatches() {
						if im != "" {
							return fmt.Errorf("a write to a row the organization does not own carried If-Match %q", im)
						}
					}
					return nil
				},
			),
		}},
	})
}

func TestRepositoryDefaultsImportInAFollowingChildSaysWhy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.DefaultsSource("parent", "Acme Parent", nil)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []tfresource.TestStep{{
			Config: defaultsConfig(f.URL, "  auto_dispatch = true") + `
import {
  to = sreagent_repository_defaults.d
  id = "repository_defaults"
}
`,
			ExpectError: regexp.MustCompile(`(?s)Nothing\s+to\s+import.*follows\s+its\s+parent`),
		}},
	})
}

// An exported file adopts with an import and plans clean afterwards.
func TestImportedRepositoryDefaultsPlanClean(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.DefaultsSource("own", "", nil)
	f.SetDefaults(map[string]any{"fix_runner": "platform", "platform_fallback": false, "auto_dispatch": false, "check_network": false, "allow_ci_config": false})
	block := `
resource "sreagent_repository_defaults" "d" {
  fix_runner        = "platform"
  platform_fallback = false
  auto_dispatch     = false
  check_network     = false
  allow_ci_config   = false
}
`
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock(f.URL) + block + `
import {
  to = sreagent_repository_defaults.d
  id = "repository_defaults"
}
`,
				ConfigPlanChecks: tfresource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{importsOnly{}}},
			},
			{Config: providerBlock(f.URL) + block, ConfigPlanChecks: emptyPlan},
		},
	})
	if n := len(defaultsPuts(f)); n != 0 {
		t.Fatalf("adopting an exported file sent %d PUTs", n)
	}
}

// Destroying the resource in a child that has its own defaults hands the child
// back to its parent, as a previewed write of follow_parent.
func TestRepositoryDefaultsDestroyInAChildFollowsTheParent(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.DefaultsSource("parent", "Acme Parent", nil)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			puts := defaultsPuts(f)
			if len(puts) < 2 {
				return fmt.Errorf("no destroy writes: %v", puts)
			}
			preview, write := puts[len(puts)-2], puts[len(puts)-1]
			if !reflect.DeepEqual(preview, map[string]any{"follow_parent": true}) || fingerprintOf(write) == "" || write["follow_parent"] != true {
				return fmt.Errorf("the destroy wrote %v then %v", preview, write)
			}
			if got := f.Defaults()["source"]; got != "parent" {
				return fmt.Errorf("the organization reads its defaults from %v", got)
			}
			return nil
		},
		Steps: []tfresource.TestStep{{Config: defaultsConfig(f.URL, "  auto_dispatch = false")}},
	})
}

func TestRepositoryDefaultsDestroyWithoutAParentLeavesThem(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			for _, p := range defaultsPuts(f) {
				if _, sent := p["follow_parent"]; sent {
					return fmt.Errorf("a destroy with no parent sent %v", p)
				}
			}
			if got := f.Defaults(); got["auto_dispatch"] != false || got["source"] != "own" {
				return fmt.Errorf("the defaults were not left in place: %v", got)
			}
			return nil
		},
		Steps: []tfresource.TestStep{{Config: defaultsConfig(f.URL, "  auto_dispatch = false")}},
	})
}

func TestRepositoryDefaultsDataSourceInAFollowingChildIsNotConfigured(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.DefaultsSource("parent", "Acme Parent", nil)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_repository_defaults\" \"d\" {}\n",
			Check: tfresource.ComposeTestCheckFunc(
				tfresource.TestCheckResourceAttr("data.sreagent_repository_defaults.d", "configured", "false"),
			),
		}},
	})
}

func TestRepositoryDefaultsDataSourceReadsTheRow(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.DefaultsSource("own", "", nil)
	f.SetDefaults(map[string]any{"auto_dispatch": false})
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_repository_defaults\" \"d\" {}\n",
			Check: tfresource.ComposeTestCheckFunc(
				tfresource.TestCheckResourceAttr("data.sreagent_repository_defaults.d", "configured", "true"),
				tfresource.TestCheckResourceAttr("data.sreagent_repository_defaults.d", "auto_dispatch", "false"),
				tfresource.TestCheckResourceAttr("data.sreagent_repository_defaults.d", "source", "own"),
				tfresource.TestCheckNoResourceAttr("data.sreagent_repository_defaults.d", "confirm"),
				tfresource.TestCheckNoResourceAttr("data.sreagent_repository_defaults.d", "preview_fingerprint"),
			),
		}},
	})
}

// A platform from before the repository defaults answers 404 to the singleton
// itself, and the resource says which release it needs.
func TestRepositoryDefaultsOnAPlatformWithoutThemNamesTheRelease(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.DefaultsSource("missing", "", nil)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []tfresource.TestStep{{
			Config:      defaultsConfig(f.URL, "  auto_dispatch = false"),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`needs\s+platform\s+v0\.477\.0\s+or\s+later`),
		}},
	})
}

// The warnings are read by a person; they are checked on the resource itself,
// because Terraform's test harness discards them.

func planDiagnostics(t *testing.T, f *fakefacade.Fake, state map[string]tftypes.Value, config map[string]tftypes.Value, plan map[string]tftypes.Value) diag.Diagnostics {
	t.Helper()
	c, err := client.New(client.Config{BaseURL: f.URL, APIKey: "sre_ak_unit", UserAgent: "test"}) //nolint:gosec // the fake facade accepts any sre_ak_ key
	if err != nil {
		t.Fatal(err)
	}
	var spec engine.Spec
	for _, s := range specs.All() {
		if s.Key == "repository_defaults" {
			spec = s
		}
	}
	r := engine.NewResource(spec)()
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: c}, &resource.ConfigureResponse{})
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	typ := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	build := func(overrides map[string]tftypes.Value) tftypes.Value {
		vals := map[string]tftypes.Value{}
		for name, at := range typ.AttributeTypes {
			vals[name] = tftypes.NewValue(at, nil)
		}
		for k, v := range overrides {
			vals[k] = v
		}
		return tftypes.NewValue(typ, vals)
	}
	req := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Raw: build(config), Schema: schemaResp.Schema},
		Plan:   tfsdk.Plan{Raw: build(plan), Schema: schemaResp.Schema},
		State:  tfsdk.State{Raw: build(state), Schema: schemaResp.Schema},
	}
	if state == nil {
		req.State = tfsdk.State{Raw: tftypes.NewValue(typ, nil), Schema: schemaResp.Schema}
	}
	resp := resource.ModifyPlanResponse{Plan: req.Plan}
	r.(resource.ResourceWithModifyPlan).ModifyPlan(context.Background(), req, &resp)
	return resp.Diagnostics
}

func str(s string) tftypes.Value   { return tftypes.NewValue(tftypes.String, s) }
func boolean(b bool) tftypes.Value { return tftypes.NewValue(tftypes.Bool, b) }

func warningText(d diag.Diagnostics) string {
	var b strings.Builder
	for _, x := range d.Warnings() {
		b.WriteString(x.Summary() + ": " + x.Detail() + "\n")
	}
	return b.String()
}

func TestAPlanNamesWhatMovesAndLeavingTheParentInTheWarning(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.DefaultsSource("parent", "Acme Parent", nil)
	for _, repo := range []string{"acme/web", "acme/api"} {
		seedRepo(t, f, repo)
	}
	f.FollowingChildren(map[string]any{"organization_id": "c1", "organization_name": "Acme EU", "moved": 4, "ci_pinned": 1, "no_runner": false})
	set := map[string]tftypes.Value{"auto_dispatch": boolean(false), "id": str("repository_defaults")}
	d := planDiagnostics(t, f, nil, set, set)
	if d.HasError() {
		t.Fatalf("the plan failed: %v", d)
	}
	text := warningText(d)
	for _, want := range []string{"auto_dispatch from true to false", "2 repositories will read a different value: acme/api, acme/web", "Child organization Acme EU", "gives it its own for all six settings", "destroy this resource to follow the parent again"} {
		if !strings.Contains(text, want) {
			t.Errorf("the warning lacks %q:\n%s", want, text)
		}
	}
	if f.Previews() != 1 {
		t.Fatalf("one plan took %d previews", f.Previews())
	}
}

func TestAPlanWithAReadOnlyKeyWarnsThatTheApplyPreviews(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.ReadOnly()
	set := map[string]tftypes.Value{"auto_dispatch": boolean(false), "id": str("repository_defaults")}
	d := planDiagnostics(t, f, nil, set, set)
	if d.HasError() {
		t.Fatalf("a read-only key must plan: %v", d)
	}
	if text := warningText(d); !strings.Contains(text, "needs an api:admin key; the apply previews it and applies what it finds") {
		t.Fatalf("the warning is %q", text)
	}
}

func TestAPlanWithAReadOnlyKeyStillRefusesATurnedOnSettingWithoutItsConfirmation(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.ReadOnly()
	set := map[string]tftypes.Value{"allow_ci_config": boolean(true), "id": str("repository_defaults")}
	d := planDiagnostics(t, f, nil, set, set)
	if !d.HasError() || !strings.Contains(fmt.Sprint(d.Errors()), "pass confirm ci_config") {
		t.Fatalf("want the confirmation refused at plan, got %v", d)
	}
}
