package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// seedOne posts a row to the fake the way the platform's own doors would
// have written it, and answers the id the API handed back. The fake stores
// what the body carries and answers a zero for everything else, so a row
// POSTed with its defaults carries them the way a real platform does.
func seedOne(t *testing.T, base, key string, row map[string]any) string {
	t.Helper()
	body, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/config/"+key, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer sre_ak_unit")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("seeding %s: status %d", key, resp.StatusCode)
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	id, _ := out.Data["id"].(string)
	return id
}

// importsOnly fails a plan on any action that is not a no-op or an import.
// The export's adoption promise is "N to import, 0 to add, 0 to change, 0
// to destroy", and both defects this test exists to catch are violations
// of exactly that: a read-only attribute written in the config fails the
// plan outright, and a null meaning written as a literal plans an update
// on the row being imported.
type importsOnly struct{}

func (importsOnly) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, c := range req.Plan.ResourceChanges {
		switch {
		case c.Change.Actions.NoOp():
		case c.Change.Actions.Create() && c.Change.Importing != nil:
		default:
			resp.Error = fmt.Errorf("%s is planned %v: the export must write only what the provider can set, at no null meaning (differing attributes %s)", c.Address, c.Change.Actions, diffKeys(c.Change.Before, c.Change.After))
			return
		}
	}
}

// requiresUpdate fails a plan unless one named resource is planned an
// update: the shape a literal at a null meaning produces on import.
type requiresUpdate struct {
	address string
}

func (r requiresUpdate) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, c := range req.Plan.ResourceChanges {
		if c.Address == r.address && c.Change.Actions.Update() {
			return
		}
	}
	resp.Error = fmt.Errorf("%s is not planned an update", r.address)
}

func diffKeys(before, after any) string {
	b, _ := before.(map[string]any)
	a, _ := after.(map[string]any)
	keys := map[string]bool{}
	for k := range b {
		keys[k] = true
	}
	for k := range a {
		keys[k] = true
	}
	out := ""
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		if !reflect.DeepEqual(b[k], a[k]) {
			out += fmt.Sprintf("%s(%v -> %v) ", k, b[k], a[k])
		}
	}
	return out
}

// TestExportAdoptsCleanly is the export round trip: rows seeded the way
// the platform answers them (defaults included), a configuration written
// the way GET /api/v1/config/export?format=hcl writes it, and two plans.
// The first must be imports only, and the second, after adoption, must
// show no changes at all. The rows carry every trap the export has: an AI
// provider at its platform defaults, a rule at the platform's own pacing,
// a webhook whose tokened URL the export never writes, and the two settings a person
// decides in the app.
func TestExportAdoptsCleanly(t *testing.T) {
	f := fakefacade.New(t, specs.All())

	provider := seedOne(t, f.URL, "ai_providers", map[string]any{
		"name": "claude", "provider": "anthropic",
		"max_tokens": 4096, "temperature": 0.7, "timeout_ms": 60000,
		"api_key": "sk-ant-export",
	})
	config := seedOne(t, f.URL, "outbound_configs", map[string]any{
		"name": "hook", "provider_type": "webhook",
		"base_url":                 "https://hooks.example.com/T0/B0/token",
		"default_severity_mapping": map[string]any{},
		"api_key":                  "whsec-export",
	})
	rule := seedOne(t, f.URL, "outbound_rules", map[string]any{
		"name": "critical", "outbound_config_id": config,
		"cooldown_minutes": 30, "step_order": 0,
		"match_labels": map[string]any{},
	})

	// A repository that holds only its runner and follows the organization default
	// for the rest, and the organization's own defaults.
	f.SetDefaults(map[string]any{"fix_runner": "platform", "platform_fallback": true, "auto_dispatch": false, "draft_prs": true, "check_network": false, "allow_ci_config": false})
	f.DefaultsSource("own", "", nil)
	repo := seedOne(t, f.URL, "repo_settings", map[string]any{"repo": "acme/web", "fix_runner": "self_hosted"})
	staging := seedOne(t, f.URL, "repo_settings", map[string]any{"repo": "acme/web", "branch": "staging"})

	// The security settings and rules, as a fresh organization and the seed hold them: the SLA with no
	// SLA for info (null), the digest with no channel (null), two marked classes, the seeded default
	// rule and a rule that pages, an allow rule that ended long ago with its repository in lower case
	// and the end time with microseconds, and one that never ends.
	f.SetAcceptedClasses("other:ssrf", "other:xxe")
	f.Mutate("security_code_classes", "security_code_classes", func(r map[string]any) {
		r["classes"] = []any{"other:ssrf", "other:xxe"}
		r["available_classes"] = []any{}
	})
	defaultRule := seedOne(t, f.URL, "security_notification_rules", map[string]any{
		"name": "Critical and high findings", "areas": []string{}, "min_severity": "high",
		"events": []string{"new_finding", "sla_breached", "acceptance_expired", "fix_verified"}, "slack": true, "email": true, "page": false,
	})
	pageRule := seedOne(t, f.URL, "security_notification_rules", map[string]any{
		"name": "Page on critical", "areas": []string{"secrets", "iam"}, "min_severity": "critical",
		"events": []string{"new_finding"}, "slack": true, "slack_channel": "#security-critical", "email": false, "page": true,
	})
	const endedRule, vendorRule = "00000000-0000-4000-8000-000000000013", "00000000-0000-4000-8000-000000000014"
	f.Seed("secret_allow_rules", map[string]any{
		"id": endedRule, "path_glob": "**/testdata/**", "reason": "Vendored test fixtures, not real keys",
		"repository": "acme/web", "detector_type": "aws_access_key", "expires_at": "2020-01-01T00:00:00.000000Z", "created_at": "2019-01-01T00:00:00.000000Z",
	})
	f.Seed("secret_allow_rules", map[string]any{
		"id": vendorRule, "path_glob": "vendor/**", "reason": "Third party code is scanned upstream",
		"repository": nil, "detector_type": nil, "expires_at": nil, "created_at": "2019-01-01T00:00:00.000000Z",
	})

	// Written the way the export writes it: the AI provider's default
	// max_tokens, temperature and timeout_ms left out (the provider reads
	// an answer equal to one back as null), the rule's default cooldown,
	// step order and empty label match left out, the webhook's empty
	// severity mapping left out, its tokened URL left out too (the platform
	// answers base_url_set and base_url_host, never the URL, and the export
	// writes only the commented base_url_wo pair), and the two settings a
	// person decides left to the app.
	resources := fmt.Sprintf(`
resource "sreagent_organization_settings" "organization_settings" {
  alert_storm_threshold           = 0
  alert_storm_window_seconds      = 0
  alert_grouping_ai_enabled       = false
  alert_refire_cooldown_minutes   = 0
  automation_approval_ttl_minutes = 0
  investigation_cooldown_minutes  = 0
  auto_open_incidents             = false
  auto_close_incidents            = false
  incident_severity_floor         = ""
  investigation_paused            = false
  monthly_ai_budget_usd           = 0
  ai_budget_warn_percent          = 0
  timezone                        = ""
}

resource "sreagent_ai_provider" "claude" {
  name               = "claude"
  provider_type      = "anthropic"
  enabled            = true
  priority           = 0
  auto_upgrade_model = false
  model_overrides    = jsonencode({})
}

resource "sreagent_outbound_config" "hook" {
  name          = "hook"
  provider_type = "webhook"
  enabled       = true
  priority      = 0
  # base_url is set on the platform and never exported. To manage it here:
  # base_url_wo         = var.hook_base_url
  # base_url_wo_version = 1
}

resource "sreagent_outbound_rule" "critical" {
  name               = "critical"
  outbound_config_id = %q
  enabled            = true
}
`, config) + `
resource "sreagent_repo_setting" "acme_web_all" {
  repo                = "acme/web"
  writable_extensions = []
  fix_runner          = "self_hosted"
  follows_default     = ["platform_fallback", "auto_dispatch", "draft_prs", "check_network", "allow_ci_config"]
}

resource "sreagent_repo_setting" "acme_web_staging" {
  repo                = "acme/web"
  branch              = "staging"
  writable_extensions = []
  follows_default     = ["draft_prs", "check_network", "allow_ci_config"]
}

resource "sreagent_repository_defaults" "repository_defaults" {
  fix_runner        = "platform"
  platform_fallback = true
  auto_dispatch     = false
  draft_prs         = true
  check_network     = false
  allow_ci_config   = false
}
` + `
resource "sreagent_security_sla_policy" "security_sla_policy" {
  critical_days = 7
  high_days     = 30
  medium_days   = 90
  low_days      = 180
  info_days     = 0
}

resource "sreagent_security_digest_settings" "security_digest_settings" {
  enabled       = false
  cadence       = "weekly"
  weekday       = 1
  hour          = 9
  slack_channel = ""
  email         = false
}

resource "sreagent_security_code_classes" "security_code_classes" {
  classes = ["other:ssrf", "other:xxe"]
}

resource "sreagent_security_notification_rule" "critical_and_high_findings" {
  name         = "Critical and high findings"
  enabled      = true
  areas        = []
  min_severity = "high"
  events       = ["new_finding", "sla_breached", "acceptance_expired", "fix_verified"]
  slack        = true
  email        = true
  page         = false
}

resource "sreagent_security_notification_rule" "page_on_critical" {
  name          = "Page on critical"
  enabled       = true
  areas         = ["secrets", "iam"]
  min_severity  = "critical"
  events        = ["new_finding"]
  slack         = true
  slack_channel = "#security-critical"
  email         = false
  page          = true
}

resource "sreagent_secret_allow_rule" "acme_web_testdata_aws_access_key" {
  path_glob     = "**/testdata/**"
  reason        = "Vendored test fixtures, not real keys"
  repository    = "acme/web"
  detector_type = "aws_access_key"
  expires_at    = "2020-01-01T00:00:00.000000Z"
}

resource "sreagent_secret_allow_rule" "all_vendor" {
  path_glob = "vendor/**"
  reason    = "Third party code is scanned upstream"
}
`

	imports := fmt.Sprintf(`
import {
  to = sreagent_organization_settings.organization_settings
  id = "organization_settings"
}

import {
  to = sreagent_ai_provider.claude
  id = %q
}

import {
  to = sreagent_outbound_config.hook
  id = %q
}

import {
  to = sreagent_outbound_rule.critical
  id = %q
}
`, provider, config, rule) + fmt.Sprintf(`
import {
  to = sreagent_repo_setting.acme_web_all
  id = %q
}

import {
  to = sreagent_repo_setting.acme_web_staging
  id = %q
}

import {
  to = sreagent_repository_defaults.repository_defaults
  id = "repository_defaults"
}
`, repo, staging) + fmt.Sprintf(`
import {
  to = sreagent_security_sla_policy.security_sla_policy
  id = "security_sla_policy"
}

import {
  to = sreagent_security_digest_settings.security_digest_settings
  id = "security_digest_settings"
}

import {
  to = sreagent_security_code_classes.security_code_classes
  id = "security_code_classes"
}

import {
  to = sreagent_security_notification_rule.critical_and_high_findings
  id = %q
}

import {
  to = sreagent_security_notification_rule.page_on_critical
  id = %q
}

import {
  to = sreagent_secret_allow_rule.acme_web_testdata_aws_access_key
  id = %q
}

import {
  to = sreagent_secret_allow_rule.all_vendor
  id = %q
}
`, defaultRule, pageRule, endedRule, vendorRule)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []resource.TestStep{
			{
				Config:           providerBlock(f.URL) + resources + imports,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{importsOnly{}}},
			},
			{
				Config:           providerBlock(f.URL) + resources,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
	puts := 0
	for _, key := range []string{"repository_defaults", "repo_settings", "security_sla_policy", "security_digest_settings", "security_code_classes", "security_notification_rules", "secret_allow_rules"} {
		puts += len(f.PutBodies(key))
	}
	if puts != 0 {
		t.Fatalf("adopting the export wrote %d times: it must import and change nothing", puts)
	}
}

// TestExportLiteralAtANullMeaningNeverAdopts pins the trap the round trip
// keeps shut: a literal written where the provider reads null (here, an AI
// provider's max_tokens at the platform default 4096) plans an update on
// the row being imported, and on every plan after that, even though the row
// never changes. This is why the export omits fields sitting at their null
// meanings, and why a provider change that makes this test flip is a
// change the export's rules must hear about.
func TestExportLiteralAtANullMeaningNeverAdopts(t *testing.T) {
	f := fakefacade.New(t, specs.All())

	provider := seedOne(t, f.URL, "ai_providers", map[string]any{
		"name": "claude", "provider": "anthropic",
		"max_tokens": 4096, "temperature": 0.7, "timeout_ms": 60000,
		"api_key": "sk-ant-export",
	})

	config := fmt.Sprintf(`
resource "sreagent_ai_provider" "claude" {
  name          = "claude"
  provider_type = "anthropic"
  max_tokens    = 4096
}

import {
  to = sreagent_ai_provider.claude
  id = %q
}
`, provider)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []resource.TestStep{
			{
				Config:           providerBlock(f.URL) + config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{requiresUpdate{address: "sreagent_ai_provider.claude"}}},
			},
		},
	})
}
