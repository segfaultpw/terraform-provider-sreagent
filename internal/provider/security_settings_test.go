package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const (
	sla    = "sreagent_security_sla_policy.p"
	digest = "sreagent_security_digest_settings.d"
	codes  = "sreagent_security_code_classes.c"
)

func noDiff() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
}

func slaConfig(f *fakefacade.Fake, body string) string {
	return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_security_sla_policy\" \"p\" {\n%s}\n", body)
}

func digestConfig(f *fakefacade.Fake, body string) string {
	return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_security_digest_settings\" \"d\" {\n%s}\n", body)
}

func codesConfig(f *fakefacade.Fake, body string) string {
	return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_security_code_classes\" \"c\" {\n%s}\n", body)
}

const slaDefaults = "  critical_days = 7\n  high_days     = 30\n  medium_days   = 90\n  low_days      = 180\n  info_days     = 0\n"

// No SLA is a value a person sets on the Settings page, stored as null. The
// resource states it as 0, which no real number of days can be, and sends null.
func TestNoSLARoundTripsAsZero(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				// The platform's defaults, info with no SLA, plan clean on a fresh organization.
				Config:           slaConfig(f, slaDefaults),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(sla, "info_days", "0"),
					resource.TestCheckResourceAttr(sla, "defaults", "true"),
					resource.TestCheckResourceAttr(sla, "id", "security_sla_policy"),
					func(*terraform.State) error {
						if v, ok := f.LastBody("PUT", "security_sla_policy")["info_days"]; !ok || v != nil {
							return fmt.Errorf("0 must go out as null, the body was %v", f.LastBody("PUT", "security_sla_policy"))
						}
						return nil
					},
				),
			},
			{
				Config:           slaConfig(f, strings.Replace(slaDefaults, "high_days     = 30", "high_days     = 0", 1)),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(sla, "high_days", "0"),
					resource.TestCheckResourceAttr(sla, "defaults", "false"),
					func(*terraform.State) error {
						body := f.LastBody("PUT", "security_sla_policy")
						if v, ok := body["high_days"]; !ok || v != nil || len(body) != 1 {
							return fmt.Errorf("only high_days changed and must go out as null, the body was %v", body)
						}
						if got := f.Row("security_sla_policy", "security_sla_policy")["high_days"]; got != nil {
							return fmt.Errorf("the platform stores null for no SLA, got %v", got)
						}
						return nil
					},
				),
			},
			{
				// Stop managing it: the data source still reads null where the resource reads 0.
				Config: providerBlock(f.URL) + "data \"sreagent_security_sla_policy\" \"p\" {}\n",
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("data.sreagent_security_sla_policy.p", "high_days"),
					resource.TestCheckResourceAttr("data.sreagent_security_sla_policy.p", "critical_days", "7"),
				),
			},
		},
	})
}

// A block that names one day sends one day, and a day set in the app since is not reverted.
func TestOmittedSLADaysAreLeftAsTheyAre(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: slaConfig(f, "  critical_days = 3\n"),
				Check: func(*terraform.State) error {
					if body := f.LastBody("PUT", "security_sla_policy"); len(body) != 1 || fmt.Sprint(body["critical_days"]) != "3" {
						return fmt.Errorf("only the named day goes out, the body was %v", body)
					}
					return nil
				},
			},
			{
				PreConfig: func() {
					f.Mutate("security_sla_policy", "security_sla_policy", func(r map[string]any) { r["high_days"] = 45 })
				},
				Config:           slaConfig(f, "  critical_days = 3\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: func(*terraform.State) error {
					if got := fmt.Sprint(f.Row("security_sla_policy", "security_sla_policy")["high_days"]); got != "45" {
						return fmt.Errorf("a day the block leaves out must stay as it is, got %s", got)
					}
					return nil
				},
			},
			// Stating the day takes it over.
			{Config: slaConfig(f, "  critical_days = 3\n  high_days = 20\n"), Check: resource.TestCheckResourceAttr(sla, "high_days", "20")},
		},
	})
}

func TestSLADaysOutsideTheRangeAreRefusedAtPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	var steps []resource.TestStep
	for _, bad := range []string{"366", "-1"} {
		steps = append(steps, resource.TestStep{Config: slaConfig(f, "  medium_days = "+bad+"\n"), PlanOnly: true,
			ExpectError: wrapped("medium_days", "must be between 1 and 365", "got "+bad)})
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: steps})
}

// Changing a day recomputes the due date of every active finding of the severity:
// the plan says so, once for each day it changes, and only when it changes.
func TestChangingTheSLAWarnsDueDatesMove(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecuritySLAPolicy, f)
	row := func(high int64) map[string]tftypes.Value {
		return map[string]tftypes.Value{"id": str("security_sla_policy"), "critical_days": tfNum(7), "high_days": tfNum(high), "medium_days": tfNum(90), "low_days": tfNum(180), "info_days": tfNum(0), "defaults": tfBool(true), "updated_at": str("x")}
	}
	state := d.value(row(30))
	if resp := d.plan(state, d.value(row(30))); resp.Diagnostics.WarningsCount() != 0 || resp.Diagnostics.HasError() {
		t.Fatalf("a plan that changes nothing says nothing: %v", resp.Diagnostics)
	}
	resp := d.plan(state, d.value(row(14)))
	if resp.Diagnostics.HasError() || !warned(resp.Diagnostics, "recomputes the due date of every active finding of this severity", "a due date a person moved is kept") {
		t.Fatalf("changing high_days must warn that due dates move: %v", resp.Diagnostics)
	}
	if n := resp.Diagnostics.WarningsCount(); n != 1 {
		t.Errorf("one day changed, one warning, got %d", n)
	}
	// Going to no SLA is a change too.
	if resp := d.plan(state, d.value(row(0))); !warned(resp.Diagnostics, "recomputes the due date") {
		t.Errorf("removing the SLA of a severity must warn: %v", resp.Diagnostics)
	}
}

// The digest at the platform's defaults has no channel, which is a value, not an omission.
func TestADigestWithoutAChannelPlansClean(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:           digestConfig(f, "  enabled       = false\n  cadence       = \"weekly\"\n  weekday       = 1\n  hour          = 9\n  slack_channel = \"\"\n  email         = false\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(digest, "slack_channel", ""),
					resource.TestCheckResourceAttr(digest, "timezone", "UTC"),
					resource.TestCheckResourceAttr(digest, "id", "security_digest_settings"),
				),
			},
			// Leaving the channel out is the same row.
			{Config: digestConfig(f, "  enabled = false\n"), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
		},
	})
}

func TestClearingTheDigestChannelSendsNull(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: digestConfig(f, "  enabled = true\n  slack_channel = \"#security\"\n"),
				Check:  resource.TestCheckResourceAttr(digest, "slack_channel", "#security"),
			},
			{
				Config:           digestConfig(f, "  enabled = false\n  slack_channel = \"\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: func(*terraform.State) error {
					body := f.LastBody("PUT", "security_digest_settings")
					if v, ok := body["slack_channel"]; !ok || v != nil {
						return fmt.Errorf("an empty channel must go out as null, the body was %v", body)
					}
					if got := f.Row("security_digest_settings", "security_digest_settings")["slack_channel"]; got != nil {
						return fmt.Errorf("the platform stores null for no channel, got %v", got)
					}
					return nil
				},
			},
		},
	})
}

func TestADigestOnWithoutADestinationFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	noWhere := wrapped("No destination for the digest", "Choose a Slack channel or email")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			// A destination left in state counts: email is on, so only enabled is stated.
			{Config: digestConfig(f, "  email = true\n")},
			{Config: digestConfig(f, "  enabled = true\n"), Check: resource.TestCheckResourceAttr(digest, "enabled", "true")},
			// Both destinations stated away.
			{Config: digestConfig(f, "  enabled = true\n  email = false\n"), PlanOnly: true, ExpectError: noWhere},
			// The destination in state goes away while the digest stays on: the channel was never there.
			{Config: digestConfig(f, "  email = false\n"), PlanOnly: true, ExpectError: noWhere},
			// A digest that is off may have none.
			{Config: digestConfig(f, "  enabled = false\n  email = false\n"), Check: resource.TestCheckResourceAttr(digest, "email", "false")},
		},
	})
}

func TestADigestOnWithoutADestinationFailsTheAdoptingPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			// A create adopts the row the organization has, which holds no destination: the plan reads it
			// and refuses, before anything is written.
			Config:      digestConfig(f, "  enabled = true\n"),
			ExpectError: wrapped("No destination for the digest", "Choose a Slack channel or email for the digest."),
		}},
	})
	if f.Calls("PUT", "security_digest_settings") != 0 {
		t.Error("a refused plan writes nothing")
	}
}

func TestDigestBoundsAreRefusedAtPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cases := []struct {
		body string
		want []string
	}{
		{"  cadence = \"hourly\"\n", []string{"cadence", "daily", "weekly"}},
		{"  weekday = 0\n", []string{"weekday", "between 1 and 7"}},
		{"  weekday = 8\n", []string{"weekday", "between 1 and 7"}},
		{"  hour = 24\n", []string{"hour", "between 0 and 23"}},
		{"  hour = -1\n", []string{"hour", "between 0 and 23"}},
		{"  slack_channel = \"security\"\n", []string{"slack_channel", "must match the pattern"}},
		{"  slack_channel = \"#a--b\"\n", []string{"slack_channel", "must match the pattern"}},
		{"  slack_channel = \"#" + strings.Repeat("a", 80) + "\"\n", []string{"slack_channel", "at most 80", "got 81"}},
	}
	var steps []resource.TestStep
	for _, c := range cases {
		steps = append(steps, resource.TestStep{Config: digestConfig(f, c.body), PlanOnly: true, ExpectError: wrapped(c.want...)})
	}
	// 80 characters is the longest channel the platform takes.
	steps = append(steps, resource.TestStep{Config: digestConfig(f, "  slack_channel = \"#"+strings.Repeat("a", 79)+"\"\n"), PlanOnly: true, ExpectNonEmptyPlan: true})
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: steps})
}

// A destroy changes nothing on the platform: a reset would be an organization-wide
// security change made by terraform destroy, and the platform has no row to delete.
func TestDestroyingTheSecuritySingletonsLeavesThemInPlace(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetAcceptedClasses("other:a")
	cfg := slaConfig(f, "  high_days = 14\n") + "\nresource \"sreagent_security_digest_settings\" \"d\" {\n  email = true\n}\n\nresource \"sreagent_security_code_classes\" \"c\" {\n  classes = [\"other:a\"]\n}\n"
	var writes int
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig: func() {
					for _, k := range []string{"security_sla_policy", "security_digest_settings", "security_code_classes"} {
						writes += f.Calls("PUT", k) + f.Calls("DELETE", k) + f.Calls("POST", k)
					}
				},
				Config: providerBlock(f.URL),
				Check: func(*terraform.State) error {
					after := 0
					for _, k := range []string{"security_sla_policy", "security_digest_settings", "security_code_classes"} {
						after += f.Calls("PUT", k) + f.Calls("DELETE", k) + f.Calls("POST", k)
					}
					if after != writes {
						return fmt.Errorf("destroying a singleton must write nothing, got %d writes", after-writes)
					}
					if got := fmt.Sprint(f.Row("security_sla_policy", "security_sla_policy")["high_days"]); got != "14" {
						return fmt.Errorf("the SLA must stay as it was, got %s", got)
					}
					if f.Row("security_digest_settings", "security_digest_settings")["email"] != true {
						return fmt.Errorf("the digest must stay as it was")
					}
					if got := fmt.Sprint(f.Row("security_code_classes", "security_code_classes")["classes"]); got != "[other:a]" {
						return fmt.Errorf("the marks must stay as they were, got %s", got)
					}
					return nil
				},
			},
		},
	})
}

func TestADestroyOfASecuritySingletonSaysItLeftTheSettingsInPlace(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecuritySLAPolicy, f)
	resp := d.destroy(d.value(map[string]tftypes.Value{"id": str("security_sla_policy"), "high_days": tfNum(14)}))
	if resp.Diagnostics.HasError() || !warned(resp.Diagnostics, "Settings left in place", "sreagent_security_sla_policy", "the platform keeps its current values") {
		t.Fatalf("the destroy must warn that the settings stay: %v", resp.Diagnostics)
	}
	for _, k := range []string{"PUT", "DELETE", "POST", "GET"} {
		if n := f.Calls(k, "security_sla_policy"); n != 0 {
			t.Errorf("a destroy must not call the platform, %s was called %d times", k, n)
		}
	}
}

func TestTheSecuritySingletonsImportByName(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetAcceptedClasses("other:a")
	cfg := slaConfig(f, "  critical_days = 5\n") +
		"\nresource \"sreagent_security_digest_settings\" \"d\" {\n  enabled = false\n}\n" +
		"\nresource \"sreagent_security_code_classes\" \"c\" {\n  classes = [\"other:a\"]\n}\n"
	steps := []resource.TestStep{{Config: cfg}}
	for _, r := range []struct{ addr, id string }{{sla, "security_sla_policy"}, {digest, "security_digest_settings"}, {codes, "security_code_classes"}} {
		steps = append(steps,
			resource.TestStep{ResourceName: r.addr, ImportState: true, ImportStateId: r.id, ImportStateVerify: true, Config: cfg},
			resource.TestStep{ResourceName: r.addr, ImportState: true, ImportStateId: "wrong", Config: cfg, ExpectError: regexp.MustCompile(`Import a singleton by its name`)},
		)
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: steps})
}

func classesDirect(d *direct, classes ...string) tftypes.Value {
	return d.value(map[string]tftypes.Value{
		"id": str("security_code_classes"), "classes": strList(classes...),
		"built_in_classes": str(`["hardcoded_secret"]`), "available_classes": str(`["other:z"]`), "limit": tfNum(50),
	})
}

// The platform stores the marks sorted and deduplicated; the list is a set to Terraform.
func TestCodeClassesInAnyOrderPlanClean(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetAcceptedClasses("other:a", "other:b")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:           codesConfig(f, "  classes = [\"other:b\", \"other:a\"]\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: func(*terraform.State) error {
					if got := fmt.Sprint(f.Row("security_code_classes", "security_code_classes")["classes"]); got != "[other:a other:b]" {
						return fmt.Errorf("the platform stores them sorted, got %s", got)
					}
					return nil
				},
			},
			{Config: codesConfig(f, "  classes = [\"other:a\", \"other:b\"]\n"), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
			{Config: codesConfig(f, "  classes = [\"other:b\", \"other:a\"]\n"), PlanOnly: true},
			{Config: codesConfig(f, "  classes = [\"other:b\", \"other:a\"]\n"), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(codes, "classes.#", "2"),
					resource.TestCheckResourceAttr(codes, "limit", "50"),
					resource.TestCheckResourceAttrSet(codes, "built_in_classes"),
					resource.TestCheckResourceAttrSet(codes, "available_classes"),
					resource.TestCheckResourceAttr(codes, "id", "security_code_classes"),
				)},
			// A block that names no classes leaves the marks as they are.
			{Config: providerBlock(f.URL) + "resource \"sreagent_security_code_classes\" \"c\" {\n}\n", ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
		},
	})
}

func TestUnmarkingAClassWarnsItsFindingsClose(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecurityCodeClasses, f)
	state := classesDirect(d, "other:a", "other:b")
	if resp := d.plan(state, classesDirect(d, "other:a", "other:b")); resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("keeping every mark says nothing: %v", resp.Diagnostics)
	}
	resp := d.plan(state, classesDirect(d, "other:a"))
	if resp.Diagnostics.HasError() || !warned(resp.Diagnostics, "Unmarking other:b closes its open security findings", "marking it again restores them") {
		t.Fatalf("unmarking a class must warn that its findings close: %v", resp.Diagnostics)
	}
	if n := resp.Diagnostics.WarningsCount(); n != 1 {
		t.Errorf("one class goes, one warning, got %d", n)
	}
	// Marking one is not a removal; unmarking every class warns for each.
	if resp := d.plan(state, classesDirect(d, "other:a", "other:b", "other:z")); resp.Diagnostics.WarningsCount() != 0 {
		t.Errorf("adding a class says nothing: %v", resp.Diagnostics)
	}
	if resp := d.plan(state, classesDirect(d)); resp.Diagnostics.WarningsCount() != 2 {
		t.Errorf("[] unmarks both: %v", resp.Diagnostics)
	}
}

func TestMarkingAClassThatIsNotAvailableFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: codesConfig(f, "  classes = []\n")},
			{Config: codesConfig(f, "  classes = [\"other:ghost\"]\n"), PlanOnly: true,
				ExpectError: wrapped("A class the organization has not accepted", "other:ghost is not a class this organization accepted", "available_classes: none.")},
			// An apply refreshes first, so the accepted classes the plan judges against are current.
			{PreConfig: func() { f.SetAcceptedClasses("other:real", "other:also") }, Config: codesConfig(f, "  classes = []\n")},
			{
				Config:      codesConfig(f, "  classes = [\"other:ghost\"]\n"),
				PlanOnly:    true,
				ExpectError: wrapped("other:ghost is not a class this organization accepted", "available_classes: other:real, other:also."),
			},
			{Config: codesConfig(f, "  classes = [\"other:real\"]\n"), Check: resource.TestCheckResourceAttr(codes, "classes.0", "other:real")},
		},
	})
}

// A class already marked stays valid when its alias is removed from the accepted ones: the
// platform keeps the mark, so a configuration that keeps it plans clean.
func TestAMarkedClassWhoseAliasWasRemovedStaysValid(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetAcceptedClasses("other:real")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: codesConfig(f, "  classes = [\"other:real\"]\n")},
			{PreConfig: func() { f.SetAcceptedClasses() }, Config: codesConfig(f, "  classes = [\"other:real\"]\n"), ConfigPlanChecks: noDiff()},
		},
	})
}

func TestMarkingABuiltInClassFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: codesConfig(f, "  classes = []\n")},
			{Config: codesConfig(f, "  classes = [\"hardcoded_secret\"]\n"), PlanOnly: true,
				ExpectError: wrapped("A built-in class is not listed", "hardcoded_secret always counts as security and is not listed", "list the other:<kind> classes you want counted.")},
		},
	})
}

func TestAnEmptyListUnmarksEveryClass(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetAcceptedClasses("other:a")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: codesConfig(f, "  classes = [\"other:a\"]\n"), Check: resource.TestCheckResourceAttr(codes, "classes.#", "1")},
			{
				Config:           codesConfig(f, "  classes = []\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(codes, "classes.#", "0"),
					func(*terraform.State) error {
						body := f.LastBody("PUT", "security_code_classes")
						if list, ok := body["classes"].([]any); !ok || len(list) != 0 {
							return fmt.Errorf("[] must be sent as an empty list, the body was %v", body)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestFiftyOneClassesAreRefusedAtPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	list := func(n int) string {
		names := make([]string, 0, n)
		for i := 0; i < n; i++ {
			names = append(names, fmt.Sprintf("%q", fmt.Sprintf("other:c%02d", i)))
		}
		return "  classes = [" + strings.Join(names, ", ") + "]\n"
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: codesConfig(f, list(51)), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)classes.*at\s+most\s+50`)},
		},
	})
}

// A create adopts the classes the organization holds, so the plan judges against them and the
// refusal comes before anything is written.
func TestAnUnavailableClassOnCreateFailsTheAdoptingPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      codesConfig(f, "  classes = [\"other:ghost\"]\n"),
			ExpectError: wrapped("other:ghost is not a class this organization accepted"),
		}},
	})
	if f.Calls("PUT", "security_code_classes") != 0 {
		t.Error("a refused plan writes nothing")
	}
}

// A destroy leaves the settings in place; what that means is not the same for each, so the
// warning says what stays in force.
func TestDestroyingASecuritySingletonSaysWhatStaysInForce(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	for _, c := range []struct {
		spec engine.Spec
		want string
	}{
		{specs.SecuritySLAPolicy, "Due dates keep the current policy."},
		{specs.SecurityDigestSettings, "The digest keeps sending while enabled is true."},
		{specs.SecurityCodeClasses, "The classes stay marked."},
	} {
		d := newDirect(t, c.spec, f)
		resp := d.destroy(d.value(map[string]tftypes.Value{"id": str(c.spec.Key)}))
		if !warned(resp.Diagnostics, "Settings left in place", "the platform keeps its current values", c.want) {
			t.Errorf("destroying sreagent_%s must say %q: %v", c.spec.TypeName, c.want, resp.Diagnostics)
		}
	}
	// A released singleton keeps the plain sentence.
	d := newDirect(t, specs.OrganizationSettings, f)
	resp := d.destroy(d.value(map[string]tftypes.Value{"id": str("organization_settings")}))
	if n := len(resp.Diagnostics.Warnings()); n != 1 || strings.Contains(resp.Diagnostics.Warnings()[0].Detail(), "keeps sending") {
		t.Errorf("a released singleton says only that the settings stay: %v", resp.Diagnostics)
	}
}

// A platform that predates a security singleton answers 404 for it. The resource says which
// release serves it, as the data source does, and a refresh never reads the 404 as a deleted row.
func TestAPlatformThatPredatesASecuritySingletonNamesTheRelease(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: slaConfig(f, "  high_days = 14\n")},
			{
				PreConfig:   func() { f.Remove("security_sla_policy", "security_sla_policy") },
				Config:      slaConfig(f, "  high_days = 14\n"),
				PlanOnly:    true,
				ExpectError: wrapped("Could not read sreagent_security_sla_policy", "sreagent_security_sla_policy needs platform v0.476.3 or later"),
			},
			{PreConfig: func() {
				f.Seed("security_sla_policy", map[string]any{"id": "security_sla_policy", "critical_days": 7, "high_days": 14, "medium_days": 90, "low_days": 180, "info_days": nil, "defaults": false, "updated_at": nil})
			}, Config: slaConfig(f, "  high_days = 14\n")},
		},
	})
}

func TestAdoptingOverAPlatformThatPredatesTheSingletonWarnsTheRelease(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Remove("security_digest_settings", "security_digest_settings")
	d := newDirect(t, specs.SecurityDigestSettings, f)
	resp := d.adopt(map[string]tftypes.Value{"enabled": tfBool(false)})
	if resp.Diagnostics.HasError() || !warned(resp.Diagnostics, "sreagent_security_digest_settings needs platform v0.476.4 or later") {
		t.Errorf("a 404 at plan names the release and does not fail the plan: %v", resp.Diagnostics)
	}
}
