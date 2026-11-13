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

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const (
	sla    = "sreagent_security_sla_policy.p"
	digest = "sreagent_security_digest_settings.d"
)

func slaConfig(f *fakefacade.Fake, body string) string {
	return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_security_sla_policy\" \"p\" {\n%s}\n", body)
}

func digestConfig(f *fakefacade.Fake, body string) string {
	return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_security_digest_settings\" \"d\" {\n%s}\n", body)
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

func TestADigestOnWithoutADestinationIsRefusedByThePlatformOnAnAdoptingCreate(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			// A create has no prior row to read, so the platform answers: the apply fails with its sentence.
			Config:      digestConfig(f, "  enabled = true\n"),
			ExpectError: wrapped("Choose a Slack channel or email for the digest."),
		}},
	})
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
	cfg := slaConfig(f, "  high_days = 14\n") + "\nresource \"sreagent_security_digest_settings\" \"d\" {\n  email = true\n}\n"
	var writes int
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig: func() {
					for _, k := range []string{"security_sla_policy", "security_digest_settings"} {
						writes += f.Calls("PUT", k) + f.Calls("DELETE", k) + f.Calls("POST", k)
					}
				},
				Config: providerBlock(f.URL),
				Check: func(*terraform.State) error {
					after := 0
					for _, k := range []string{"security_sla_policy", "security_digest_settings"} {
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
	cfg := slaConfig(f, "  critical_days = 5\n") +
		"\nresource \"sreagent_security_digest_settings\" \"d\" {\n  enabled = false\n}\n"
	steps := []resource.TestStep{{Config: cfg}}
	for _, r := range []struct{ addr, id string }{{sla, "security_sla_policy"}, {digest, "security_digest_settings"}} {
		steps = append(steps,
			resource.TestStep{ResourceName: r.addr, ImportState: true, ImportStateId: r.id, ImportStateVerify: true, Config: cfg},
			resource.TestStep{ResourceName: r.addr, ImportState: true, ImportStateId: "wrong", Config: cfg, ExpectError: regexp.MustCompile(`Import a singleton by its name`)},
		)
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: steps})
}
