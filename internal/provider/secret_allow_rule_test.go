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

const allow = "sreagent_secret_allow_rule.a"

func allowConfig(f *fakefacade.Fake, body string) string {
	return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_secret_allow_rule\" \"a\" {\n  path_glob = \"**/testdata/**\"\n  reason    = \"Vendored test fixtures, not real keys\"\n%s}\n", body)
}

func replaced() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(allow, plancheck.ResourceActionDestroyBeforeCreate)}}
}

func TestSecretAllowRuleLifecycle(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := allowConfig(f, "  repository    = \"Tf/Acc\"\n  detector_type = \"aws_access_key\"\n  expires_at    = \"2099-01-01T00:00:00Z\"\n")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(allow, "path_glob", "**/testdata/**"),
					resource.TestCheckResourceAttr(allow, "repository", "Tf/Acc"),
					resource.TestCheckResourceAttr(allow, "detector_type", "aws_access_key"),
					resource.TestCheckResourceAttrSet(allow, "created_at"),
					resource.TestCheckResourceAttrSet(allow, "id"),
				),
			},
			// The platform stores the repository in lower case and the time with microseconds; an
			// import reads those spellings.
			{ResourceName: allow, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"repository", "expires_at"}, Config: cfg},
			{
				Config: providerBlock(f.URL),
				Check: func(*terraform.State) error {
					if n := f.Calls("DELETE", "secret_allow_rules"); n != 1 || f.Rows("secret_allow_rules") != 0 {
						return fmt.Errorf("destroy must delete the rule once, deletes %d, rows %d", n, f.Rows("secret_allow_rules"))
					}
					return nil
				},
			},
		},
	})
}

func TestAllowRuleRepositoryCaseIsNotDrift(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	var id string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:           allowConfig(f, "  repository = \"Acme/Web\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: func(s *terraform.State) error {
					id = s.RootModule().Resources[allow].Primary.ID
					if got := f.Row("secret_allow_rules", id)["repository"]; got != "acme/web" {
						return fmt.Errorf("the platform stores the repository in lower case, got %v", got)
					}
					return nil
				},
			},
			{
				Config:           allowConfig(f, "  repository = \"Acme/Web\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: func(s *terraform.State) error {
					if got := s.RootModule().Resources[allow].Primary.ID; got != id {
						return fmt.Errorf("the rule must not have been replaced, id %s became %s", id, got)
					}
					return nil
				},
			},
			// The lower case spelling is the same repository: state takes the spelling in place, and
			// nothing is sent or replaced.
			{
				Config: allowConfig(f, "  repository = \"acme/web\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(allow, plancheck.ResourceActionUpdate),
				}},
				Check: func(s *terraform.State) error {
					if got := s.RootModule().Resources[allow].Primary.ID; got != id {
						return fmt.Errorf("the rule must not have been replaced")
					}
					if n := f.Calls("POST", "secret_allow_rules") + f.Calls("PUT", "secret_allow_rules") + f.Calls("DELETE", "secret_allow_rules"); n != 1 {
						return fmt.Errorf("only the first create writes, got %d writes", n)
					}
					return nil
				},
			},
		},
	})
}

func TestAllowRuleExpiryInAnyRFC3339FormIsNotDrift(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	var id string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:           allowConfig(f, "  expires_at = \"2099-01-01T00:00:00Z\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: func(s *terraform.State) error {
					id = s.RootModule().Resources[allow].Primary.ID
					if got := f.Row("secret_allow_rules", id)["expires_at"]; got != "2099-01-01T00:00:00.000000Z" {
						return fmt.Errorf("the platform answers microseconds, got %v", got)
					}
					return nil
				},
			},
			// The same instant in another zone is the same rule.
			{
				Config:           allowConfig(f, "  expires_at = \"2099-01-01T01:00:00+01:00\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: func(s *terraform.State) error {
					if got := s.RootModule().Resources[allow].Primary.ID; got != id {
						return fmt.Errorf("the same instant must not replace the rule")
					}
					return nil
				},
			},
			{Config: allowConfig(f, "  expires_at = \"2099-01-01T01:00:00+01:00\"\n"), PlanOnly: true},
			// Another instant is another rule.
			{Config: allowConfig(f, "  expires_at = \"2099-06-01T00:00:00Z\"\n"), ConfigPlanChecks: replaced()},
		},
	})
}

// The rule has no update door, so a removed scope must not silently stay: the replacement
// covers every repository, as the block now says.
func TestRemovingTheRepositoryReplacesTheRule(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	var first string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: allowConfig(f, "  repository = \"acme/web\"\n  detector_type = \"aws_access_key\"\n  expires_at = \"2099-01-01T00:00:00Z\"\n"),
				Check:  func(s *terraform.State) error { first = s.RootModule().Resources[allow].Primary.ID; return nil },
			},
			{
				Config:           allowConfig(f, "  detector_type = \"aws_access_key\"\n  expires_at = \"2099-01-01T00:00:00Z\"\n"),
				ConfigPlanChecks: replaced(),
				Check: func(s *terraform.State) error {
					id := s.RootModule().Resources[allow].Primary.ID
					if id == first {
						return fmt.Errorf("the rule must have been replaced")
					}
					if f.Row("secret_allow_rules", first) != nil {
						return fmt.Errorf("the old, narrower rule must be gone")
					}
					if got := f.Row("secret_allow_rules", id)["repository"]; got != nil {
						return fmt.Errorf("the new rule covers every repository, got %v", got)
					}
					return nil
				},
			},
			// The same for the detector and the end.
			{Config: allowConfig(f, "  expires_at = \"2099-01-01T00:00:00Z\"\n"), ConfigPlanChecks: replaced(),
				Check: func(s *terraform.State) error {
					if got := f.Row("secret_allow_rules", s.RootModule().Resources[allow].Primary.ID)["detector_type"]; got != nil {
						return fmt.Errorf("the new rule covers every kind, got %v", got)
					}
					return nil
				}},
			{Config: allowConfig(f, ""), ConfigPlanChecks: replaced(),
				Check: func(s *terraform.State) error {
					if got := f.Row("secret_allow_rules", s.RootModule().Resources[allow].Primary.ID)["expires_at"]; got != nil {
						return fmt.Errorf("the new rule never ends, got %v", got)
					}
					return nil
				}},
		},
	})
}

func TestChangingTheReasonReplacesTheRule(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(reason string) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_secret_allow_rule\" \"a\" {\n  path_glob = \"vendor/**\"\n  reason = %q\n}\n", reason)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg("Vendored fixtures, not real keys")},
			{Config: cfg("Vendored fixtures, still not real keys"), ConfigPlanChecks: replaced()},
			{Config: providerBlock(f.URL) + "resource \"sreagent_secret_allow_rule\" \"a\" {\n  path_glob = \"vendor/lib/**\"\n  reason = \"Vendored fixtures, still not real keys\"\n}\n", ConfigPlanChecks: replaced()},
		},
	})
}

func TestAnAllowRuleThatAlreadyEndedFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: allowConfig(f, "  expires_at = \"2020-01-01T00:00:00Z\"\n"), PlanOnly: true, ExpectError: wrapped("expires_at must be in the future")},
			{Config: allowConfig(f, "  expires_at = \"2099-01-01T00:00:00Z\"\n")},
			// A replacement that ends in the past is refused too, before the old rule is deleted.
			{Config: allowConfig(f, "  expires_at = \"2020-01-01T00:00:00Z\"\n"), PlanOnly: true, ExpectError: wrapped("expires_at must be in the future")},
			{Config: allowConfig(f, "  expires_at = \"2099-01-01T00:00:00Z\"\n")},
		},
	})
}

func allowDirect(expires string) map[string]tftypes.Value {
	return map[string]tftypes.Value{"id": str("r1"), "path_glob": str("vendor/**"), "reason": str("Vendored fixtures"), "expires_at": str(expires), "created_at": str("2026-01-01T00:00:00.000000Z")}
}

// A rule that ended covers nothing. Terraform says so on every plan and plans nothing:
// the rule is still the rule the configuration describes.
func TestAnEndedRuleWarnsAndPlansNothing(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecretAllowRules, f)
	ended := d.value(allowDirect("2020-01-01T00:00:00.000000Z"))
	resp := d.plan(ended, d.value(allowDirect("2020-01-01T00:00:00Z")))
	if resp.Diagnostics.HasError() {
		t.Fatalf("a rule that ended is not an error: %v", resp.Diagnostics)
	}
	if !warned(resp.Diagnostics, "This allow rule has ended", "covers nothing", "the weekly scan files its matches again", "replace the rule with a later expires_at") {
		t.Fatalf("an ended rule must say so: %v", resp.Diagnostics)
	}
	live := d.value(allowDirect("2099-01-01T00:00:00.000000Z"))
	if resp := d.plan(live, d.value(allowDirect("2099-01-01T00:00:00Z"))); resp.Diagnostics.WarningsCount() != 0 || resp.Diagnostics.HasError() {
		t.Errorf("a rule that has not ended says nothing: %v", resp.Diagnostics)
	}
	// Moving the end to another past time is a replacement that the platform would refuse.
	if resp := d.plan(ended, d.value(allowDirect("2021-01-01T00:00:00Z"))); !resp.Diagnostics.HasError() {
		t.Errorf("a new end in the past must fail the plan: %v", resp.Diagnostics)
	}
}

func TestAnImportedEndedRulePlansClean(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Seed("secret_allow_rules", map[string]any{
		"id": "11111111-1111-1111-1111-111111111111", "path_glob": "**/testdata/**", "reason": "Vendored test fixtures, not real keys",
		"repository": nil, "detector_type": nil, "expires_at": "2020-01-01T00:00:00.000000Z", "created_at": "2019-01-01T00:00:00.000000Z",
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:           allowConfig(f, "  expires_at = \"2020-01-01T00:00:00Z\"\n") + "import {\n  to = sreagent_secret_allow_rule.a\n  id = \"11111111-1111-1111-1111-111111111111\"\n}\n",
			ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
		}},
	})
}

func TestAnAllowRuleIsRefusedAtPlanForEachBound(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cases := []struct {
		body string
		want []string
	}{
		{"  reason = \"too short\"\n", []string{"reason"}},
		{"  repository = \"acme\"\n", []string{"repository", "must match the pattern"}},
		{"  repository = \"acme/web \"\n", []string{"repository", "must match the pattern"}},
		{"  detector_type = \"Not-A-Type\"\n", []string{"detector_type", "must match the pattern"}},
		{"  detector_type = \"" + strings.Repeat("a", 51) + "\"\n", []string{"detector_type", "at most 50"}},
		{"  expires_at = \"2099-01-01\"\n", []string{"Not a time with a zone"}},
		{"  expires_at = \"tomorrow\"\n", []string{"Not a time with a zone"}},
	}
	var steps []resource.TestStep
	for _, c := range cases {
		cfg := allowConfig(f, c.body)
		if strings.HasPrefix(c.body, "  reason") {
			cfg = providerBlock(f.URL) + "resource \"sreagent_secret_allow_rule\" \"a\" {\n  path_glob = \"x/**\"\n" + c.body + "}\n"
		}
		steps = append(steps, resource.TestStep{Config: cfg, PlanOnly: true, ExpectError: wrapped(c.want...)})
	}
	steps = append(steps,
		resource.TestStep{Config: providerBlock(f.URL) + "resource \"sreagent_secret_allow_rule\" \"a\" {\n  path_glob = \"" + strings.Repeat("a", 201) + "\"\n  reason = \"Vendored test fixtures\"\n}\n", PlanOnly: true, ExpectError: wrapped("path_glob", "1 to 200 characters", "got 201")},
		resource.TestStep{Config: providerBlock(f.URL) + "resource \"sreagent_secret_allow_rule\" \"a\" {\n  path_glob = \" x/**\"\n  reason = \"Vendored test fixtures\"\n}\n", PlanOnly: true, ExpectError: wrapped("Surrounding whitespace")},
		resource.TestStep{Config: providerBlock(f.URL) + "resource \"sreagent_secret_allow_rule\" \"a\" {\n  path_glob = \"x/**\"\n  reason = \"" + strings.Repeat("é", 500) + "\"\n}\n"},
	)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: steps})
}

func TestThe101stAllowRuleFailsTheApplyWithThePlatformsSentence(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + `
resource "sreagent_secret_allow_rule" "many" {
  count     = 101
  path_glob = "dir-${count.index}/**"
  reason    = "Vendored test fixtures"
}
`,
			ExpectError: wrapped("At most 100 allow rules can be kept."),
		}},
	})
}

// The platform refuses a reason that quotes a credential, and nothing of it is kept.
func TestAReasonThatQuotesACredentialIsRefusedByThePlatformAndKeptNowhere(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(f.URL) + "resource \"sreagent_secret_allow_rule\" \"a\" {\n  path_glob = \"x/**\"\n  reason = \"The key AKIAIOSFODNN7EXAMPLE is a documentation key\"\n}\n",
			ExpectError: regexp.MustCompile(`(?s)looks\s+like\s+it\s+holds\s+a\s+credential`),
		}},
	})
	if f.Rows("secret_allow_rules") != 0 {
		t.Error("a refused rule must leave no row")
	}
}

func allowFields(over map[string]tftypes.Value) map[string]tftypes.Value {
	row := allowDirect("2020-01-01T00:00:00.000000Z")
	for k, v := range over {
		row[k] = v
	}
	return row
}

// A rule that ended and whose other field is edited is replaced, and the platform refuses a
// rule that ends in the past: the old rule would be deleted first and the create would fail.
// The plan says so, with the end date and the ways out.
func TestAnEndedRuleThatIsEditedFailsThePlanNamingTheEndAndTheExits(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecretAllowRules, f)
	ended := d.value(allowFields(nil))
	for name, edit := range map[string]map[string]tftypes.Value{
		"reason":        {"reason": str("Vendored fixtures, reworded")},
		"path_glob":     {"path_glob": str("vendor/lib/**")},
		"repository":    {"repository": str("acme/web")},
		"detector_type": {"detector_type": str("aws_access_key")},
	} {
		plan := allowFields(edit)
		plan["expires_at"] = str("2020-01-01T00:00:00Z")
		resp := d.plan(ended, d.value(plan))
		if !resp.Diagnostics.HasError() || !warnedOrErrored(resp.Diagnostics, "2020-01-01T00:00:00Z", "replaces the rule", name, "set a later expires_at", "remove the block") {
			t.Errorf("editing %s of an ended rule must fail the plan with the end date and the exits: %v", name, resp.Diagnostics)
		}
	}
	// The same rule in another spelling is not an edit: a case-only repository and the same
	// instant in another zone are warned about, never refused.
	state := d.value(allowFields(map[string]tftypes.Value{"repository": str("acme/web")}))
	same := allowFields(map[string]tftypes.Value{"repository": str("Acme/Web")})
	same["expires_at"] = str("2020-01-01T01:00:00+01:00")
	if resp := d.plan(state, d.value(same)); resp.Diagnostics.HasError() || !warned(resp.Diagnostics, "This allow rule has ended") {
		t.Errorf("a respelled rule is the same rule and only warns: %v", resp.Diagnostics)
	}
	// An edit that also moves the end into the future is a replacement the platform accepts.
	fixed := allowFields(map[string]tftypes.Value{"reason": str("Vendored fixtures, reworded")})
	fixed["expires_at"] = str("2099-01-01T00:00:00Z")
	if resp := d.plan(ended, d.value(fixed)); resp.Diagnostics.HasError() {
		t.Errorf("a later expires_at is the way out: %v", resp.Diagnostics)
	}
	// Dropping the end is the other.
	never := allowFields(map[string]tftypes.Value{"reason": str("Vendored fixtures, reworded"), "expires_at": tftypes.NewValue(tftypes.String, nil)})
	if resp := d.plan(ended, d.value(never)); resp.Diagnostics.HasError() {
		t.Errorf("a rule that never ends is accepted: %v", resp.Diagnostics)
	}
}

func TestAnImportedEndedRuleThatIsEditedFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Seed("secret_allow_rules", map[string]any{
		"id": "11111111-1111-1111-1111-111111111111", "path_glob": "**/testdata/**", "reason": "Vendored test fixtures, not real keys",
		"repository": nil, "detector_type": nil, "expires_at": "2020-01-01T00:00:00.000000Z", "created_at": "2019-01-01T00:00:00.000000Z",
	})
	imp := "import {\n  to = sreagent_secret_allow_rule.a\n  id = \"11111111-1111-1111-1111-111111111111\"\n}\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: allowConfig(f, "  expires_at = \"2020-01-01T00:00:00Z\"\n") + imp, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
			{
				Config:      providerBlock(f.URL) + "resource \"sreagent_secret_allow_rule\" \"a\" {\n  path_glob = \"**/testdata/**\"\n  reason = \"Vendored test fixtures, reworded\"\n  expires_at = \"2020-01-01T00:00:00Z\"\n}\n",
				PlanOnly:    true,
				ExpectError: wrapped("2020-01-01T00:00:00Z", "replaces the rule", "set a later expires_at", "remove the block"),
			},
			{
				// The refused plan deleted nothing: the old rule is still the one the configuration describes.
				Config: allowConfig(f, "  expires_at = \"2020-01-01T00:00:00Z\"\n"),
				Check: func(*terraform.State) error {
					if f.Rows("secret_allow_rules") != 1 || f.Calls("DELETE", "secret_allow_rules") != 0 {
						return fmt.Errorf("a refused plan must leave the old rule where it was, rows %d, deletes %d", f.Rows("secret_allow_rules"), f.Calls("DELETE", "secret_allow_rules"))
					}
					return nil
				},
			},
		},
	})
}

// Removing a scope replaces the rule with a wider or longer lived one, and the plan says what widens.
func TestRemovingAScopeFromAnAllowRuleWarnsWhatWidens(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecretAllowRules, f)
	full := allowFields(map[string]tftypes.Value{"repository": str("acme/web"), "detector_type": str("aws_access_key"), "expires_at": str("2099-01-01T00:00:00.000000Z")})
	state := d.value(full)
	keep := func() map[string]tftypes.Value {
		m := map[string]tftypes.Value{}
		for k, v := range full {
			m[k] = v
		}
		return m
	}
	if resp := d.plan(state, d.value(keep())); resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("keeping every scope says nothing: %v", resp.Diagnostics)
	}
	for attr, want := range map[string][]string{
		"repository":    {"Removing repository", "every repository"},
		"detector_type": {"Removing detector_type", "every kind"},
		"expires_at":    {"Removing expires_at", "never ends"},
	} {
		plan := keep()
		plan[attr] = tftypes.NewValue(tftypes.String, nil)
		resp := d.plan(state, d.value(plan))
		if resp.Diagnostics.HasError() {
			t.Fatalf("removing %s is allowed: %v", attr, resp.Diagnostics)
		}
		if !warned(resp.Diagnostics, want...) {
			t.Errorf("removing %s must warn %v: %v", attr, want, resp.Diagnostics)
		}
		if n := resp.Diagnostics.WarningsCount(); n != 1 {
			t.Errorf("removing %s says one thing, got %d warnings: %v", attr, n, resp.Diagnostics)
		}
	}
	// Changing a scope to another value is not a removal.
	plan := keep()
	plan["repository"] = str("acme/api")
	if resp := d.plan(state, d.value(plan)); resp.Diagnostics.WarningsCount() != 0 {
		t.Errorf("another repository is not a wider rule: %v", resp.Diagnostics)
	}
}
