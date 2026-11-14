package provider

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// Turning auto-merge on needs the platform's confirmation, and the plan says so
// before anything is written: the platform would refuse the apply in the same
// words.
func TestTurningAutoMergeOnWithoutConfirmFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      repoSettingConfig(f.URL, "  auto_merge = true"),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`(?s)Confirmation\s+needed.*Turning\s+on\s+auto-merge\s+needs\s+confirmation:\s+pass\s+confirm\s+auto_merge.*confirm\s*=\s*\["auto_merge"\]`),
		}},
	})
	if n := f.Calls("POST", "repo_settings"); n != 0 {
		t.Fatalf("a refused plan still sent %d POSTs", n)
	}
}

// With the confirmation the row is created holding auto_merge, the confirmation
// travels with the write and is never stored, and a configuration that keeps it
// plans nothing afterwards.
func TestAutoMergeOnWithConfirmApplies(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := repoSettingConfig(f.URL, "  auto_merge = true\n  confirm = [\"auto_merge\"]")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoSetting, "auto_merge", "true"),
					resource.TestCheckResourceAttr(repoSetting, "set_here.#", "1"),
					resource.TestCheckResourceAttr(repoSetting, "set_here.0", "auto_merge"),
					resource.TestCheckNoResourceAttr(repoSetting, "confirm"),
					func(*terraform.State) error {
						if got := f.LastBody("POST", "repo_settings")["confirm"]; !reflect.DeepEqual(got, []any{"auto_merge"}) {
							return fmt.Errorf("the create sent confirm %v", got)
						}
						return nil
					},
				),
			},
			{Config: cfg, ConfigPlanChecks: emptyPlan},
			{ResourceName: repoSetting, ImportState: true, ImportStateVerify: true, Config: cfg, ImportStateVerifyIdentifierAttribute: "id"},
		},
	})
}

// Off needs nothing: turning it off, or never turning it on, is not confirmed.
func TestAutoMergeOffNeedsNoConfirmation(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  auto_merge = false"), Check: resource.TestCheckResourceAttr(repoSetting, "auto_merge", "false")},
			{Config: repoSettingConfig(f.URL, "  service = \"api\""), Check: resource.TestCheckResourceAttr(repoSetting, "auto_merge", "false")},
		},
	})
}

// An update that turns it on is confirmed like a create, and one that leaves the
// setting out leaves it as it is.
func TestAutoMergeUpdateNeedsTheConfirmationAndLeavingItOutKeepsIt(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  auto_merge = false")},
			{
				Config:      repoSettingConfig(f.URL, "  auto_merge = true"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Confirmation\s+needed.*pass\s+confirm\s+auto_merge`),
			},
			{
				Config: repoSettingConfig(f.URL, "  auto_merge = true\n  confirm = [\"auto_merge\"]"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoSetting, "auto_merge", "true"),
					putBodyIs(f, map[string]any{"auto_merge": true, "confirm": []any{"auto_merge"}}),
				),
			},
			{
				Config: repoSettingConfig(f.URL, "  service = \"api\""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoSetting, "auto_merge", "true"),
					resource.TestCheckResourceAttr(repoSetting, "set_here.#", "1"),
				),
			},
			{
				Config: repoSettingConfig(f.URL, "  auto_merge = false"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoSetting, "auto_merge", "false"),
					putBodyIs(f, map[string]any{"auto_merge": false}),
				),
			},
		},
	})
}

// A confirmation names a change, it never makes one: adding or dropping it alone
// plans nothing and sends nothing.
func TestAutoMergeConfirmAloneNeverWrites(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  service = \"api\"")},
			{
				Config:           repoSettingConfig(f.URL, "  service = \"api\"\n  confirm = [\"auto_merge\"]"),
				ConfigPlanChecks: emptyPlan,
				Check:            noPut(f),
			},
		},
	})
}

// A stated value equal to what the row already reads is not written: a row that
// follows a default that says on needs no confirmation to be stated on, and
// stays following.
func TestAutoMergeStatedAsTheFollowedDefaultIsNotPinned(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetDefaults(map[string]any{"auto_merge": true})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  service = \"api\""), Check: resource.TestCheckResourceAttr(repoSetting, "auto_merge", "true")},
			{
				Config:           repoSettingConfig(f.URL, "  service = \"api\"\n  auto_merge = true"),
				ConfigPlanChecks: emptyPlan,
				Check: resource.ComposeTestCheckFunc(
					noPut(f),
					resource.TestCheckResourceAttr(repoSetting, "set_here.#", "0"),
				),
			},
		},
	})
}

// follows_default names auto_merge like any other setting: the row's own value
// goes back on the default, with no confirmation when the default says off.
func TestFollowsDefaultPutsAutoMergeBackOnTheDefault(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: repoSettingConfig(f.URL, "  auto_merge = true\n  confirm = [\"auto_merge\"]"),
				Check:  resource.TestCheckResourceAttr(repoSetting, "set_here.0", "auto_merge"),
			},
			{
				Config: repoSettingConfig(f.URL, "  follows_default = [\"auto_merge\"]"),
				Check: resource.ComposeTestCheckFunc(
					putBodyIs(f, map[string]any{"follows_default": []any{"auto_merge"}}),
					resource.TestCheckResourceAttr(repoSetting, "auto_merge", "false"),
					resource.TestCheckResourceAttr(repoSetting, "set_here.#", "0"),
					resource.TestCheckResourceAttr(repoSetting, "following.#", "7"),
				),
			},
			{Config: repoSettingConfig(f.URL, "  follows_default = [\"auto_merge\"]"), ConfigPlanChecks: emptyPlan},
		},
	})
}

// A reset onto a default that says on turns auto-merge on for the row, which the
// plan cannot see: the platform refuses it, the refusal shows as a diagnostic
// with its own sentence, and the confirmation lets it through.
func TestFollowsDefaultOntoADefaultThatSaysOnNeedsTheConfirmation(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  auto_merge = false")},
			{
				PreConfig:   func() { f.SetDefaults(map[string]any{"auto_merge": true}) },
				Config:      repoSettingConfig(f.URL, "  follows_default = [\"auto_merge\"]"),
				ExpectError: regexp.MustCompile(`(?s)Turning\s+on\s+auto-merge\s+for\s+acme/api\s+needs\s+confirmation:\s+pass\s+confirm\s+auto_merge`),
			},
			{
				Config: repoSettingConfig(f.URL, "  follows_default = [\"auto_merge\"]\n  confirm = [\"auto_merge\"]"),
				Check: resource.ComposeTestCheckFunc(
					putBodyIs(f, map[string]any{"follows_default": []any{"auto_merge"}, "confirm": []any{"auto_merge"}}),
					resource.TestCheckResourceAttr(repoSetting, "auto_merge", "true"),
					resource.TestCheckResourceAttr(repoSetting, "set_here.#", "0"),
				),
			},
		},
	})
}

// A branch row created beside a reset that the create cannot carry is reset by
// the follow-up write, which carries the confirmation too.
func TestACreateThatResetsAutoMergeOntoADefaultThatSaysOnCarriesTheConfirmation(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetDefaults(map[string]any{"auto_merge": true})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: repoSettingConfig(f.URL, "  follows_default = [\"auto_merge\"]\n  confirm = [\"auto_merge\"]"),
			Check:  resource.TestCheckResourceAttr(repoSetting, "auto_merge", "true"),
		}},
	})
}

// The platform refuses to remove an all-branches entry that says off while the
// default says on: the destroy fails with its sentence, nothing is deleted, and
// turning the default off lets it go.
func TestAnAutoMergeDeleteThePlatformRefusesShowsItsSentence(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: repoSettingConfig(f.URL, "  auto_merge = false")},
			{
				PreConfig:   func() { f.SetDefaults(map[string]any{"auto_merge": true}) },
				Config:      providerBlock(f.URL),
				ExpectError: regexp.MustCompile(`(?s)merge\s+by\s+themselves.*Turn\s+the\s+organization\s+default\s+off\s+first`),
			},
			{
				PreConfig: func() {
					f.SetDefaults(map[string]any{"auto_merge": false})
					if f.Row("repo_settings", "00000000-0000-0000-0000-000000000001") == nil {
						t.Error("the refused delete removed the row")
					}
				},
				Config: providerBlock(f.URL),
			},
		},
	})
}

// A platform from before auto-merge answers no such key: it reads as null and the
// rest of the row is unaffected.
func TestAutoMergeReadsAnOlderPlatformWithoutIt(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: repoSettingConfig(f.URL, "  service = \"api\""),
				Check: func(*terraform.State) error {
					f.Mutate("repo_settings", "00000000-0000-0000-0000-000000000001", func(r map[string]any) { delete(r, "auto_merge") })
					return nil
				},
			},
			{Config: repoSettingConfig(f.URL, "  service = \"api\""), ConfigPlanChecks: emptyPlan},
		},
	})
}

// ---- the organization's repository defaults ----

func TestTurningAutoMergeOnForTheDefaultsWithoutConfirmFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      defaultsConfig(f.URL, "  auto_merge = true"),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`(?s)Confirmation\s+needed.*Turning\s+on\s+auto-merge\s+for\s+every\s+repository.*needs\s+confirmation:\s+pass\s+confirm\s+auto_merge.*confirm\s*=\s*\["auto_merge"\]`),
		}},
	})
	if n := len(defaultsPuts(f)); n != 1 {
		t.Fatalf("the failing plan sent %d PUTs, want its one preview", n)
	}
}

func TestTurningAutoMergeOnForTheDefaultsWithConfirmApplies(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := defaultsConfig(f.URL, "  auto_merge = true\n  confirm = [\"auto_merge\"]")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(repoDefaults, "auto_merge", "true"),
					resource.TestCheckNoResourceAttr(repoDefaults, "confirm"),
					func(*terraform.State) error {
						puts := defaultsPuts(f)
						write := puts[len(puts)-1]
						if !reflect.DeepEqual(write["confirm"], []any{"auto_merge"}) || write["auto_merge"] != true {
							return fmt.Errorf("the write was %v", write)
						}
						return nil
					},
				),
			},
			{Config: cfg, ConfigPlanChecks: emptyPlan},
			{ResourceName: repoDefaults, ImportState: true, ImportStateId: "repository_defaults", ImportStateVerify: true, ImportStateVerifyIdentifierAttribute: "id", ImportStateVerifyIgnore: []string{"preview_fingerprint"}, Config: cfg},
			{Config: defaultsConfig(f.URL, "  auto_merge = false"), Check: resource.TestCheckResourceAttr(repoDefaults, "auto_merge", "false")},
		},
	})
}

// The plan names what the change starts: the repositories whose pull requests
// would start merging by themselves, and those that open as drafts and never do.
func TestTheDefaultsPlanNamesTheRepositoriesAutoMergeStartsOn(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	for _, repo := range []string{"acme/web", "acme/api"} {
		seedRepo(t, f, repo)
	}
	plan := map[string]tftypes.Value{"auto_merge": boolean(true), "id": str("repository_defaults")}
	config := map[string]tftypes.Value{
		"auto_merge": boolean(true), "id": str("repository_defaults"),
		"confirm": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{str("auto_merge")}),
	}
	d := planDiagnostics(t, f, config, plan)
	if d.HasError() {
		t.Fatalf("the plan failed: %v", d)
	}
	text := warningText(d)
	for _, want := range []string{"auto_merge from false to true", "start to merge by themselves once the checks their branch requires pass, on: acme/api (", "acme/web ("} {
		if !strings.Contains(text, want) {
			t.Errorf("the warning lacks %q:\n%s", want, text)
		}
	}
}

func TestADefaultsPreviewWithNoAutoMergeKeyStillWarns(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.DropPreviewKey("auto_merge")
	seedRepo(t, f, "acme/web")
	set := map[string]tftypes.Value{"auto_dispatch": boolean(false), "id": str("repository_defaults")}
	d := planDiagnostics(t, f, set, set)
	if d.HasError() {
		t.Fatalf("an older platform's preview must read: %v", d)
	}
	if text := warningText(d); !strings.Contains(text, "auto_dispatch from true to false") || strings.Contains(text, "merge by themselves") {
		t.Fatalf("the warning is %q", text)
	}
}

func TestAChildDestroyThatWouldTurnAutoMergeOnNeedsConfirmOnDestroy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.DefaultsSource("parent", "Acme Parent", map[string]any{"auto_merge": true})
	seedRepo(t, f, "acme/web")
	own := "  auto_dispatch = false\n  auto_merge = false"
	confirmed := own + "\n  confirm_on_destroy = [\"auto_merge\"]"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: defaultsConfig(f.URL, own)},
			{
				Config: defaultsConfig(f.URL, own), Destroy: true,
				ExpectError: regexp.MustCompile(`(?s)Confirmation\s+needed\s+to\s+destroy.*confirm_on_destroy\s+=\s+\["auto_merge"\]`),
			},
			{Config: defaultsConfig(f.URL, confirmed), Check: resource.TestCheckResourceAttr(repoDefaults, "confirm_on_destroy.0", "auto_merge")},
			{Config: defaultsConfig(f.URL, confirmed), Destroy: true},
		},
	})
	if got := f.Defaults()["source"]; got != "parent" {
		t.Fatalf("the destroy did not hand the defaults back: %v", got)
	}
}

// ---- the organization's GitHub settings ----

const githubSettings = "sreagent_github_settings.g"

func githubSettingsConfig(url, body string) string {
	return providerBlock(url) + fmt.Sprintf("resource \"sreagent_github_settings\" \"g\" {\n%s\n}\n", body)
}

func TestGitHubSettingsAutoMergeRequiresVerificationRoundTrips(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: githubSettingsConfig(f.URL, "  auto_merge_requires_verification = true"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(githubSettings, "auto_merge_requires_verification", "true"),
					func(*terraform.State) error {
						if got := f.Row("github_settings", "github_settings")["auto_merge_requires_verification"]; got != true {
							return fmt.Errorf("the platform holds %v", got)
						}
						return nil
					},
				),
			},
			{Config: githubSettingsConfig(f.URL, "  auto_merge_requires_verification = true"), ConfigPlanChecks: emptyPlan},
			{
				// Leaving it out leaves it as it is.
				Config: githubSettingsConfig(f.URL, "  draft_prs = true"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(githubSettings, "auto_merge_requires_verification", "true"),
					func(*terraform.State) error {
						if _, sent := f.LastBody("PUT", "github_settings")["auto_merge_requires_verification"]; sent {
							return fmt.Errorf("an unset auto_merge_requires_verification was sent: %v", f.LastBody("PUT", "github_settings"))
						}
						return nil
					},
				),
			},
			{ResourceName: githubSettings, ImportState: true, ImportStateId: "github_settings", ImportStateVerify: true, Config: githubSettingsConfig(f.URL, "  draft_prs = true")},
			{Config: githubSettingsConfig(f.URL, "  auto_merge_requires_verification = false"), Check: resource.TestCheckResourceAttr(githubSettings, "auto_merge_requires_verification", "false")},
			{
				Config:  providerBlock(f.URL),
				Destroy: false,
				Check: func(*terraform.State) error {
					if f.Row("github_settings", "github_settings")["auto_merge_requires_verification"] != false {
						return fmt.Errorf("destroying the settings must leave the platform's value in place")
					}
					return nil
				},
			},
		},
	})
}

func TestGitHubSettingsAutoMergeRequiresVerificationIsReadByTheDataSource(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("github_settings", "github_settings", func(r map[string]any) { r["auto_merge_requires_verification"] = true })
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_github_settings\" \"g\" {}\n",
			Check:  resource.TestCheckResourceAttr("data.sreagent_github_settings.g", "auto_merge_requires_verification", "true"),
		}},
	})
}
