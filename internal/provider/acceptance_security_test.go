package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
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
