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
	defer resp.Body.Close()
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

func (importsOnly) CheckPlan(ctx context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
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

func (r requiresUpdate) CheckPlan(ctx context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
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
// a webhook whose URL is held in a variable, and the two settings a person
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

	// Written the way the export writes it: the AI provider's default
	// max_tokens, temperature and timeout_ms left out (the provider reads
	// an answer equal to one back as null), the rule's default cooldown,
	// step order and empty label match left out, the webhook's empty
	// severity mapping left out, its tokened URL held in a variable (with
	// no sensitive marking: the marking propagates into the planned value,
	// and the imported state is not marked, so the first plan would report
	// an update with no visible diff), and the two settings a person
	// decides left to the app.
	resources := fmt.Sprintf(`
variable "hook_base_url" {
  type = string

  # The export writes no default; a .tfvars file or the environment holds
  # the value, and the test stands in for one here.
  default = "https://hooks.example.com/T0/B0/token"
}

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
  base_url      = var.hook_base_url
}

resource "sreagent_outbound_rule" "critical" {
  name               = "critical"
  outbound_config_id = %q
  enabled            = true
}
`, config)

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
`, provider, config, rule)

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
