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

const rule = "sreagent_security_notification_rule.r"

func ruleConfig(f *fakefacade.Fake, body string) string {
	return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_security_notification_rule\" \"r\" {\n%s}\n", body)
}

func TestNotificationRuleLifecycle(t *testing.T) {
	runLifecycle(t, lifecycle{
		spec:   specs.SecurityNotificationRules,
		create: "  name          = \"Critical findings\"\n  min_severity  = \"critical\"\n  page          = true\n  slack_channel = \"#security-alerts\"\n  events        = [\"new_finding\", \"sla_breached\"]\n",
		update: "  name          = \"Critical findings\"\n  min_severity  = \"critical\"\n  page          = true\n  slack_channel = \"#security-oncall\"\n  email         = false\n  areas         = [\"iam\", \"secrets\"]\n  events        = [\"new_finding\"]\n",
		after: map[string]string{
			"name": "Critical findings", "min_severity": "critical", "page": "true", "slack_channel": "#security-alerts",
			"enabled": "true", "slack": "true", "email": "true", "areas.#": "0", "events.#": "2", "events.1": "sla_breached",
		},
		browserEdit: func(r map[string]any) { r["slack_channel"] = "#elsewhere" },
	})
}

// A rule that leaves an attribute out gets the platform's default, so removing page = true
// from the block stops the paging instead of leaving it on with nothing in the diff.
func TestARuleLeftWithoutPageStopsPaging(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:           ruleConfig(f, "  name = \"r\"\n  min_severity = \"critical\"\n  page = true\n  email = false\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check:            resource.TestCheckResourceAttr(rule, "page", "true"),
			},
			{
				Config:           ruleConfig(f, "  name = \"r\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rule, "page", "false"),
					resource.TestCheckResourceAttr(rule, "min_severity", "high"),
					resource.TestCheckResourceAttr(rule, "email", "true"),
					func(*terraform.State) error {
						body := f.LastBody("PUT", "security_notification_rules")
						if body["page"] != false || body["min_severity"] != "high" || body["email"] != true {
							return fmt.Errorf("the platform's defaults must be sent for what the block left out, the body was %v", body)
						}
						return nil
					},
				),
			},
		},
	})
}

// A create names the platform's defaults for what it leaves out, in the order of the published
// create schema, and the next plan has nothing to say.
func TestARuleLeftBareTakesEveryPlatformDefault(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:           ruleConfig(f, "  name = \"bare\"\n"),
			ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(rule, "enabled", "true"),
				resource.TestCheckResourceAttr(rule, "areas.#", "0"),
				resource.TestCheckResourceAttr(rule, "min_severity", "high"),
				resource.TestCheckResourceAttr(rule, "events.#", "1"),
				resource.TestCheckResourceAttr(rule, "events.0", "new_finding"),
				resource.TestCheckResourceAttr(rule, "slack", "true"),
				resource.TestCheckNoResourceAttr(rule, "slack_channel"),
				resource.TestCheckResourceAttr(rule, "email", "true"),
				resource.TestCheckResourceAttr(rule, "page", "false"),
				resource.TestCheckResourceAttrSet(rule, "updated_at"),
			),
		}},
	})
}

func TestPagingWithoutCriticalFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	want := wrapped("Paging needs a critical rule", "Paging is only allowed on a rule whose minimum severity is critical", `min_severity = "critical"`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			// min_severity left out is high.
			{Config: ruleConfig(f, "  name = \"r\"\n  page = true\n"), PlanOnly: true, ExpectError: want},
			{Config: ruleConfig(f, "  name = \"r\"\n  page = true\n  min_severity = \"medium\"\n"), PlanOnly: true, ExpectError: want},
			{Config: ruleConfig(f, "  name = \"r\"\n  page = true\n  min_severity = \"critical\"\n"), Check: resource.TestCheckResourceAttr(rule, "page", "true")},
		},
	})
}

func TestARuleWithNoDestinationFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: ruleConfig(f, "  name = \"r\"\n  slack = false\n  email = false\n"), PlanOnly: true, ExpectError: wrapped("No destination", "Choose at least one of Slack, email or paging.")},
			// Paging alone is a destination.
			{Config: ruleConfig(f, "  name = \"r\"\n  slack = false\n  email = false\n  page = true\n  min_severity = \"critical\"\n"), Check: resource.TestCheckResourceAttr(rule, "slack", "false")},
		},
	})
}

// The plan check judges the plan, so a value that comes from a default or from state counts:
// the configuration here states only the severity, and page is false by default, not by
// the configuration's silence.
func TestAnUpdateThatLeavesPagingOnAHighRuleFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecurityNotificationRules, f)
	row := func(severity string, page bool, slack bool) tftypes.Value {
		return d.value(map[string]tftypes.Value{
			"id": str("r1"), "name": str("r"), "enabled": tfBool(true), "areas": strList(), "min_severity": str(severity),
			"events": strList("new_finding"), "slack": tfBool(slack), "email": tfBool(false), "page": tfBool(page),
		})
	}
	state := row("critical", true, true)
	resp := d.plan(state, row("high", true, true))
	if !resp.Diagnostics.HasError() || !strings.Contains(fmt.Sprint(resp.Diagnostics), "Paging needs a critical rule") {
		t.Fatalf("a plan that keeps paging on a high rule must fail: %v", resp.Diagnostics)
	}
	if resp := d.plan(state, row("critical", true, true)); resp.Diagnostics.HasError() {
		t.Fatalf("a critical rule may page: %v", resp.Diagnostics)
	}
	if resp := d.plan(state, row("critical", true, false)); resp.Diagnostics.HasError() {
		t.Fatalf("paging alone is a destination: %v", resp.Diagnostics)
	}
	none := d.plan(row("critical", false, true), row("critical", false, false))
	if !none.Diagnostics.HasError() || !strings.Contains(fmt.Sprint(none.Diagnostics), "No destination") {
		t.Fatalf("a plan with nowhere to send must fail: %v", none.Diagnostics)
	}
}

func TestAnUpdateThatLeavesPagingOnAHighRuleFailsThePlanThroughTerraform(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: ruleConfig(f, "  name = \"r\"\n  page = true\n  min_severity = \"critical\"\n")},
			{Config: ruleConfig(f, "  name = \"r\"\n  page = true\n  min_severity = \"high\"\n"), PlanOnly: true, ExpectError: wrapped("Paging needs a critical rule")},
			{Config: ruleConfig(f, "  name = \"r\"\n  page = true\n  min_severity = \"critical\"\n")},
		},
	})
}

func TestClearingTheChannelSendsNull(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: ruleConfig(f, "  name = \"r\"\n  slack_channel = \"#alerts\"\n"), Check: resource.TestCheckResourceAttr(rule, "slack_channel", "#alerts")},
			{
				Config:           ruleConfig(f, "  name = \"r\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(rule, "slack_channel"),
					func(*terraform.State) error {
						if v, ok := f.LastBody("PUT", "security_notification_rules")["slack_channel"]; !ok || v != nil {
							return fmt.Errorf("removing the channel must send null, the body was %v", f.LastBody("PUT", "security_notification_rules"))
						}
						return nil
					},
				),
			},
		},
	})
}

func TestARuleNameWithSurroundingSpaceIsRefused(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: ruleConfig(f, "  name = \" padded\"\n"), PlanOnly: true, ExpectError: wrapped("Surrounding whitespace", "padded")},
			{Config: ruleConfig(f, "  name = \"\"\n"), PlanOnly: true, ExpectError: wrapped("name", "must be 1 to 80 characters", "got 0")},
		},
	})
}

// The platform counts characters, not bytes: 80 two-byte characters is a name.
func TestARuleNameOfEightyCharactersIsAccepted(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: ruleConfig(f, "  name = \""+strings.Repeat("é", 81)+"\"\n"), PlanOnly: true, ExpectError: wrapped("must be 1 to 80 characters", "got 81")},
			{
				Config:           ruleConfig(f, "  name = \""+strings.Repeat("é", 80)+"\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func TestAnUnknownAreaIsRefusedAtPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cases := []struct{ body, attr string }{
		{"  areas = [\"servers\"]\n", "areas"},
		{"  events = [\"deleted\"]\n", "events"},
		{"  events = []\n", "events"},
		{"  min_severity = \"urgent\"\n", "min_severity"},
		{"  slack_channel = \"alerts\"\n", "slack_channel"},
		{"  slack_channel = \"\"\n", "slack_channel"},
		{"  slack_channel = \"#" + strings.Repeat("a", 80) + "\"\n", "slack_channel"},
	}
	var steps []resource.TestStep
	for _, c := range cases {
		steps = append(steps, resource.TestStep{Config: ruleConfig(f, "  name = \"r\"\n"+c.body), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)` + c.attr)})
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: steps})
}

// The platform allows 20 rules; Terraform sees one block at a time, so the 21st is the
// platform's to refuse at apply, in its own words.
func TestThe21stRuleFailsTheApplyWithThePlatformsSentence(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	many := func(n int) string {
		return providerBlock(f.URL) + fmt.Sprintf(`
resource "sreagent_security_notification_rule" "many" {
  count = %d
  name  = "rule-${count.index}"
}
`, n)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: many(21), ExpectError: wrapped("An organization can have at most 20 notification rules.")},
			// Twenty of them were created (which 20 depends on the order of the parallel creates).
			{
				Config: many(20),
				Check: func(*terraform.State) error {
					if n := f.Rows("security_notification_rules"); n != 20 {
						return fmt.Errorf("20 rules fit, got %d", n)
					}
					return nil
				},
			},
		},
	})
	if n := f.Rows("security_notification_rules"); n != 0 {
		t.Errorf("destroy deletes every rule, %d are left", n)
	}
}

func TestADuplicateRuleNameSaysToImport(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	seedOne(t, f.URL, "security_notification_rules", map[string]any{"name": "Taken"})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      ruleConfig(f, "  name = \"Taken\"\n"),
			ExpectError: wrapped("Import it instead of creating it", "A notification rule with that name already exists."),
		}},
	})
}

func TestDestroyingARuleDeletesIt(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: ruleConfig(f, "  name = \"r\"\n")},
			{
				Config: providerBlock(f.URL),
				Check: func(*terraform.State) error {
					if n := f.Calls("DELETE", "security_notification_rules"); n != 1 {
						return fmt.Errorf("one DELETE expected, got %d", n)
					}
					if n := f.Rows("security_notification_rules"); n != 0 {
						return fmt.Errorf("the rule must be gone, %d are left", n)
					}
					return nil
				},
			},
		},
	})
}

// The default rule the platform creates for an organization is managed by importing it: the
// block that states every field plans clean, and one that leaves events out says it will
// narrow them to the platform's default.
func TestTheSeededRuleImportsAndPlansCleanWhenEveryFieldIsStated(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	id := seedOne(t, f.URL, "security_notification_rules", map[string]any{
		"name": "Critical and high findings", "areas": []string{}, "min_severity": "high",
		"events": []string{"new_finding", "sla_breached", "acceptance_expired", "fix_verified"}, "slack": true, "email": true, "page": false,
	})
	imp := fmt.Sprintf("import {\n  to = sreagent_security_notification_rule.r\n  id = %q\n}\n", id)
	full := "  name         = \"Critical and high findings\"\n  enabled      = true\n  areas        = []\n  min_severity = \"high\"\n  events       = [\"new_finding\", \"sla_breached\", \"acceptance_expired\", \"fix_verified\"]\n  slack        = true\n  email        = true\n  page         = false\n"
	var posts int
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				PreConfig:        func() { posts = f.Calls("PUT", "security_notification_rules") },
				Config:           ruleConfig(f, full) + imp,
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: func(*terraform.State) error {
					if f.Calls("PUT", "security_notification_rules") != posts {
						return fmt.Errorf("adopting a rule that already says what the block says writes nothing")
					}
					return nil
				},
			},
			// A block that leaves events out is a change to the default event.
			{Config: ruleConfig(f, "  name = \"Critical and high findings\"\n"), Check: resource.TestCheckResourceAttr(rule, "events.#", "1")},
		},
	})
}
