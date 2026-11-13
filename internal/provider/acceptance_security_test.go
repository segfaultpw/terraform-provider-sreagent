package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

// The security settings and rules need the infrastructure suite, so every test here runs
// as the seeded organization on the plan that has it. They run against the real platform
// (acceptance/seed.sh), never in the unit suite: the unit suite proves the same lifecycles
// against the fake facade, and these keep the fake honest.

var accSkipBelow112 = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)}

// accRow reads one row of a resource key from the real platform.
func accRow(t *testing.T, path string) map[string]any {
	t.Helper()
	out, err := accClient(t).Do(context.Background(), client.Request{Method: http.MethodGet, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err := json.Unmarshal(out.Data, &row); err != nil {
		t.Fatal(err)
	}
	return row
}

// accSecurityEmpty is the plan every step of the security tests ends on: nothing to do.
var accSecurityEmpty = resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}

// The SLA policy and the digest are singletons: a day or a field left out is left as it is,
// 0 and "" are the two nulls that are values, and putting the platform's defaults back is
// stating them.
func TestAccSecuritySettings(t *testing.T) {
	accPreCheck(t)
	const sla, digest = "sreagent_security_sla_policy.test", "sreagent_security_digest_settings.test"
	cfg := func(slaBody, digestBody string) string {
		return accProvider() +
			fmt.Sprintf("resource \"sreagent_security_sla_policy\" \"test\" {\n%s\n}\n", slaBody) +
			fmt.Sprintf("resource \"sreagent_security_digest_settings\" \"test\" {\n%s\n}\n", digestBody)
	}
	defaults := "critical_days = 7\nhigh_days = 30\nmedium_days = 90\nlow_days = 180\ninfo_days = 0"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   accSkipBelow112,
		Steps: []resource.TestStep{
			{
				Config:           cfg("high_days = 14\ninfo_days = 0", "enabled = false"),
				ConfigPlanChecks: accSecurityEmpty,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(sla, "high_days", "14"),
					resource.TestCheckResourceAttr(sla, "info_days", "0"),
					func(*terraform.State) error {
						row := accRow(t, "security_sla_policy")
						if fmt.Sprint(row["high_days"]) != "14" || row["info_days"] != nil {
							return fmt.Errorf("the platform must hold 14 days for high and no SLA (null) for info, found %v", row)
						}
						return nil
					},
				),
			},
			{ResourceName: sla, ImportState: true, ImportStateId: "security_sla_policy", ImportStateVerify: true, Config: cfg("high_days = 14\ninfo_days = 0", "enabled = false")},
			// The digest weekly by email, then off again.
			{
				Config:           cfg("high_days = 14\ninfo_days = 0", "enabled = true\ncadence = \"weekly\"\nweekday = 2\nhour = 7\nemail = true\nslack_channel = \"\""),
				ConfigPlanChecks: accSecurityEmpty,
				Check:            resource.TestCheckResourceAttr(digest, "email", "true"),
			},
			{ResourceName: digest, ImportState: true, ImportStateId: "security_digest_settings", ImportStateVerify: true, Config: cfg("high_days = 14\ninfo_days = 0", "enabled = true\ncadence = \"weekly\"\nweekday = 2\nhour = 7\nemail = true\nslack_channel = \"\"")},
			{Config: cfg(defaults, "enabled = false\nemail = false\ncadence = \"weekly\"\nweekday = 1\nhour = 9\nslack_channel = \"\""), ConfigPlanChecks: accSecurityEmpty,
				Check: func(*terraform.State) error {
					if row := accRow(t, "security_sla_policy"); row["defaults"] != true {
						return fmt.Errorf("stating the defaults puts the platform's policy back, found %v", row)
					}
					return nil
				}},
			// Stating a day outside 1 to 365 is refused at plan.
			{Config: cfg("high_days = 400", "enabled = false"), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)high_days.*between\s+1\s+and\s+365`)},
			{Config: cfg(defaults, "enabled = false")},
		},
	})
	for _, k := range []string{"security_sla_policy", "security_digest_settings"} {
		if row := accRow(t, k); row == nil {
			t.Errorf("destroying the resource must leave %s in place", k)
		}
	}
}

// The code classes are a set: the list marks exactly those, in any order, and [] unmarks them
// all. The seed accepts the class other:tf_acc.
func TestAccSecurityCodeClasses(t *testing.T) {
	accPreCheck(t)
	const codes = "sreagent_security_code_classes.test"
	cfg := func(list string) string {
		return accProvider() + fmt.Sprintf("resource \"sreagent_security_code_classes\" \"test\" {\n  classes = %s\n}\n", list)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   accSkipBelow112,
		Steps: []resource.TestStep{
			{
				Config:           cfg(`["other:tf_acc"]`),
				ConfigPlanChecks: accSecurityEmpty,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(codes, "classes.#", "1"),
					resource.TestCheckResourceAttr(codes, "classes.0", "other:tf_acc"),
					resource.TestCheckResourceAttr(codes, "limit", "50"),
				),
			},
			{ResourceName: codes, ImportState: true, ImportStateId: "security_code_classes", ImportStateVerify: true, Config: cfg(`["other:tf_acc"]`)},
			// A class the organization never accepted, and a built-in one, are refused at plan.
			{Config: cfg(`["other:tf_acc", "other:never_accepted"]`), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)other:never_accepted\s+is\s+not\s+a\s+class\s+this\s+organization\s+accepted`)},
			{Config: cfg(`["hardcoded_secret"]`), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)hardcoded_secret\s+always\s+counts\s+as\s+security`)},
			{
				Config:           cfg(`[]`),
				ConfigPlanChecks: accSecurityEmpty,
				Check: func(*terraform.State) error {
					if marked, _ := accRow(t, "security_code_classes")["classes"].([]any); len(marked) != 0 {
						return fmt.Errorf("[] must unmark every class, the platform still marks %v", marked)
					}
					return nil
				},
			},
		},
	})
}

// A rule that leaves an attribute out takes the platform's default, a cleared channel sends
// null, paging needs a critical rule, and destroying the rule deletes it.
func TestAccSecurityNotificationRule(t *testing.T) {
	accPreCheck(t)
	const addr = "sreagent_security_notification_rule.test"
	cfg := func(body string) string {
		return accProvider() + fmt.Sprintf("resource \"sreagent_security_notification_rule\" \"test\" {\n  name = \"acc-rule\"\n%s}\n", body)
	}
	var id string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   accSkipBelow112,
		Steps: []resource.TestStep{
			{
				Config:           cfg("  slack_channel = \"#acc-security\"\n  events = [\"new_finding\", \"sla_breached\"]\n"),
				ConfigPlanChecks: accSecurityEmpty,
				Check: func(s *terraform.State) error {
					id = s.RootModule().Resources[addr].Primary.ID
					return nil
				},
			},
			{Config: cfg("  events = [\"new_finding\", \"sla_breached\"]\n"), ConfigPlanChecks: accSecurityEmpty, Check: resource.TestCheckNoResourceAttr(addr, "slack_channel")},
			{
				Config:           cfg("  min_severity = \"critical\"\n  page = true\n"),
				ConfigPlanChecks: accSecurityEmpty,
				Check:            resource.TestCheckResourceAttr(addr, "page", "true"),
			},
			{Config: cfg("  page = true\n"), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)Paging\s+needs\s+a\s+critical\s+rule`)},
			{ResourceName: addr, ImportState: true, ImportStateVerify: true, Config: cfg("  min_severity = \"critical\"\n  page = true\n")},
			{
				// Back to the defaults: page goes with the severity.
				Config:           cfg(""),
				ConfigPlanChecks: accSecurityEmpty,
				Check:            resource.TestCheckResourceAttr(addr, "page", "false"),
			},
			{
				Config: accProvider(),
				Check: func(*terraform.State) error {
					if _, err := accClient(t).Do(context.Background(), client.Request{Method: http.MethodGet, Path: "security_notification_rules/" + id}); !client.IsNotFound(err) {
						return fmt.Errorf("destroying a rule must delete it: %w", err)
					}
					return nil
				},
			},
		},
	})
}

// An allow rule cannot be edited: a change replaces it, the platform's spelling of the
// repository and the time is no change, and removing a scope replaces the rule with a wider one.
func TestAccSecretAllowRule(t *testing.T) {
	accPreCheck(t)
	const addr = "sreagent_secret_allow_rule.test"
	cfg := func(reason, rest string) string {
		return accProvider() + fmt.Sprintf("resource \"sreagent_secret_allow_rule\" \"test\" {\n  path_glob = \"acc/**\"\n  reason = %q\n%s}\n", reason, rest)
	}
	scoped := "  repository = \"Tf/Acc\"\n  expires_at = \"2099-12-31T00:00:00Z\"\n"
	var id string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   accSkipBelow112,
		Steps: []resource.TestStep{
			{
				Config:           cfg("Acceptance fixtures, not real keys", scoped),
				ConfigPlanChecks: accSecurityEmpty,
				Check: func(s *terraform.State) error {
					id = s.RootModule().Resources[addr].Primary.ID
					if got := accRow(t, "secret_allow_rules/"+id)["repository"]; got != "tf/acc" {
						return fmt.Errorf("the platform stores the repository in lower case, found %v", got)
					}
					return nil
				},
			},
			{
				// A change replaces the rule.
				Config: cfg("Acceptance fixtures, still not real keys", scoped),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(addr, plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: func(s *terraform.State) error {
					if s.RootModule().Resources[addr].Primary.ID == id {
						return fmt.Errorf("the rule must have been replaced")
					}
					return nil
				},
			},
			{ResourceName: addr, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"repository", "expires_at"}, Config: cfg("Acceptance fixtures, still not real keys", scoped)},
			{
				// Removing the scope replaces the rule with one that covers every repository and never ends.
				Config: cfg("Acceptance fixtures, still not real keys", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(addr, plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: func(s *terraform.State) error {
					row := accRow(t, "secret_allow_rules/"+s.RootModule().Resources[addr].Primary.ID)
					if row["repository"] != nil || row["expires_at"] != nil {
						return fmt.Errorf("the new rule covers every repository and never ends, found %v", row)
					}
					return nil
				},
			},
			{Config: cfg("Acceptance fixtures", "  expires_at = \"2020-01-01T00:00:00Z\"\n"), PlanOnly: true, ExpectError: regexp.MustCompile(`expires_at\s+must\s+be\s+in\s+the\s+future`)},
		},
	})
}

// securityAccConfigs is a configuration for each of the five security resources, for the
// tests that hold all of them to the same rule. The names are resource types, not credentials.
//
//nolint:gosec // resource types and HCL, no credential
var securityAccConfigs = map[string]string{
	"sreagent_security_sla_policy":        "critical_days = 7",
	"sreagent_security_digest_settings":   "enabled = false",
	"sreagent_security_code_classes":      "classes = []",
	"sreagent_security_notification_rule": "name = \"acc-readonly\"",
	"sreagent_secret_allow_rule":          "path_glob = \"acc/**\"\nreason = \"Acceptance fixtures, not real keys\"",
}

// A CI job holding only the api:config_read key plans the five and cannot apply them.
func TestAccSecurityResourcesReadOnlyKeyPlans(t *testing.T) {
	accPreCheck(t)
	if os.Getenv("SREAGENT_READ_API_KEY") == "" {
		t.Skip("SREAGENT_READ_API_KEY is not set; acceptance/seed.sh prints it")
	}
	admin := newCLI(t)
	t.Cleanup(func() { _, _ = admin.run("destroy", "-auto-approve") })
	var body strings.Builder
	for typeName, config := range securityAccConfigs {
		fmt.Fprintf(&body, "resource %q \"test\" {\n%s\n}\n", typeName, config)
	}
	admin.write(body.String())
	if out, err := admin.run("apply", "-auto-approve"); err != nil {
		t.Fatal(out)
	}
	reader := &cli{t: t, dir: admin.dir, env: append(append([]string{}, admin.env...), "SREAGENT_API_KEY="+os.Getenv("SREAGENT_READ_API_KEY"))}
	if out, err := reader.run("plan", "-detailed-exitcode"); err != nil {
		t.Fatalf("a read-only key must plan cleanly against unchanged rows: %s", out)
	}
	admin.write(strings.Replace(body.String(), "critical_days = 7", "critical_days = 8", 1))
	if out, err := reader.run("apply", "-auto-approve"); err == nil || !regexp.MustCompile(`can plan but not apply`).MatchString(out) {
		t.Fatalf("a read-only key must be refused on apply with the read-only diagnostic, got: %v %s", err, out)
	}
}

// An organization whose plan has the configuration API but not the infrastructure suite gets
// the platform's 403 for every verb of the five: the apply says so, naming the feature.
// The seed prints that organization and its key when it can make the plan; without them the
// test says so and skips.
func TestAccSecurityResourcesRefusedForThePlan(t *testing.T) {
	accPreCheck(t)
	key, slug := os.Getenv("SREAGENT_NOINFRA_API_KEY"), os.Getenv("SREAGENT_NOINFRA_ORGANIZATION")
	if key == "" || slug == "" {
		t.Skip("SREAGENT_NOINFRA_API_KEY and SREAGENT_NOINFRA_ORGANIZATION are not set; the seed prints them when it can make a plan with the configuration API and without the infrastructure suite")
	}
	provider := fmt.Sprintf("provider \"sreagent\" {\n  base_url     = %q\n  organization = %q\n  api_key      = %q\n}\n", os.Getenv("SREAGENT_BASE_URL"), slug, key)
	for typeName, config := range securityAccConfigs {
		t.Run(typeName, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				TerraformVersionChecks:   accSkipBelow112,
				Steps: []resource.TestStep{{
					Config:      provider + fmt.Sprintf("resource %q \"test\" {\n%s\n}\n", typeName, config),
					ExpectError: wrapped("Could not create "+typeName, "The infrastructure suite is not included in your plan.", "needs the infrastructure suite in the organization's plan", "terraform state rm "+typeName),
				}},
			})
		})
	}
}
