package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

func TestSecuritySLAPolicyDataSourceReadsTheDays(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("security_sla_policy", "security_sla_policy", func(r map[string]any) {
		r["critical_days"] = 7
		r["high_days"] = 30
		r["medium_days"] = nil
		r["low_days"] = nil
		r["info_days"] = nil
		r["defaults"] = false
	})
	const addr = "data.sreagent_security_sla_policy.p"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_security_sla_policy\" \"p\" {}\n",
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(addr, "critical_days", "7"),
				resource.TestCheckResourceAttr(addr, "high_days", "30"),
				resource.TestCheckNoResourceAttr(addr, "medium_days"),
				resource.TestCheckNoResourceAttr(addr, "info_days"),
				resource.TestCheckResourceAttr(addr, "defaults", "false"),
				resource.TestCheckResourceAttr(addr, "configured", "true"),
			),
		}},
	})
}

func TestSecurityDigestSettingsAndCodeClassesDataSources(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("security_digest_settings", "security_digest_settings", func(r map[string]any) {
		r["enabled"] = true
		r["cadence"] = "weekly"
		r["weekday"] = 1
		r["hour"] = 9
		r["slack_channel"] = "#security"
		r["timezone"] = "UTC"
	})
	f.Mutate("security_code_classes", "security_code_classes", func(r map[string]any) {
		r["classes"] = []any{"other:ssrf"}
		r["available_classes"] = []any{"other:ssrf", "other:xxe"}
		r["limit"] = 50
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + `
data "sreagent_security_digest_settings" "d" {}
data "sreagent_security_code_classes" "c" {}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.sreagent_security_digest_settings.d", "cadence", "weekly"),
				resource.TestCheckResourceAttr("data.sreagent_security_digest_settings.d", "hour", "9"),
				resource.TestCheckResourceAttr("data.sreagent_security_digest_settings.d", "slack_channel", "#security"),
				resource.TestCheckResourceAttr("data.sreagent_security_code_classes.c", "classes.#", "1"),
				resource.TestCheckResourceAttr("data.sreagent_security_code_classes.c", "classes.0", "other:ssrf"),
				resource.TestCheckResourceAttr("data.sreagent_security_code_classes.c", "limit", "50"),
				resource.TestCheckResourceAttrSet("data.sreagent_security_code_classes.c", "available_classes"),
			),
		}},
	})
}

func TestCodeDetectionSettingsDataSource(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("code_detection_settings", "code_detection_settings", func(r map[string]any) {
		r["weekly_enabled"] = true
		r["run_day"] = 2
		r["run_hour"] = 4
		r["default_classes"] = []any{"sql_injection"}
		r["manual_runs_today"] = 1
		r["jitter_minutes"] = 17
	})
	const addr = "data.sreagent_code_detection_settings.c"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_code_detection_settings\" \"c\" {}\n",
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(addr, "weekly_enabled", "true"),
				resource.TestCheckResourceAttr(addr, "run_day", "2"),
				resource.TestCheckResourceAttr(addr, "default_classes.0", "sql_injection"),
				resource.TestCheckResourceAttr(addr, "manual_runs_today", "1"),
				resource.TestCheckResourceAttr(addr, "jitter_minutes", "17"),
			),
		}},
	})
}

func TestSecurityNotificationRulesListsEveryRule(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Seed("security_notification_rules", map[string]any{
		"id": "11111111-1111-1111-1111-111111111111", "name": "Critical and high", "enabled": true,
		"areas": []any{"secrets", "iam"}, "min_severity": "high", "events": []any{"new_finding"},
		"slack": true, "slack_channel": nil, "email": true, "page": false, "updated_at": "2026-11-01T00:00:00Z",
	})
	f.Seed("security_notification_rules", map[string]any{
		"id": "22222222-2222-2222-2222-222222222222", "name": "Page on critical", "enabled": true,
		"areas": []any{}, "min_severity": "critical", "events": []any{"new_finding", "sla_breached"},
		"slack": true, "slack_channel": "#alerts-critical", "email": false, "page": true, "updated_at": nil,
	})
	const addr = "data.sreagent_security_notification_rules.all"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_security_notification_rules\" \"all\" {}\n",
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(addr, "items.#", "2"),
				resource.TestCheckResourceAttr(addr, "items.0.name", "Critical and high"),
				resource.TestCheckResourceAttr(addr, "items.0.areas.1", "iam"),
				resource.TestCheckResourceAttr(addr, "items.1.page", "true"),
				resource.TestCheckResourceAttr(addr, "items.1.slack_channel", "#alerts-critical"),
				resource.TestCheckResourceAttr(addr, "items.1.events.1", "sla_breached"),
				resource.TestCheckResourceAttr(addr, "truncated", "false"),
			),
		}},
	})
}

func TestSecretAllowRulesListsEveryRule(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Seed("secret_allow_rules", map[string]any{
		"id": "33333333-3333-3333-3333-333333333333", "path_glob": "vendor/**", "reason": "vendored fixtures, not real keys",
		"repository": "acme/web", "detector_type": nil, "expires_at": nil, "created_at": "2026-11-01T00:00:00Z",
	})
	const addr = "data.sreagent_secret_allow_rules.all"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_secret_allow_rules\" \"all\" {}\n",
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(addr, "items.#", "1"),
				resource.TestCheckResourceAttr(addr, "items.0.path_glob", "vendor/**"),
				resource.TestCheckResourceAttr(addr, "items.0.repository", "acme/web"),
				resource.TestCheckNoResourceAttr(addr, "items.0.detector_type"),
			),
		}},
	})
}

// A platform from before a security setting answers 404 for it. The singleton
// names the release it needs instead of reading as an empty answer.
func TestASecuritySettingOnAPlatformWithoutItNamesTheRelease(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Remove("security_sla_policy", "security_sla_policy")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(f.URL) + "data \"sreagent_security_sla_policy\" \"p\" {}\n",
			ExpectError: regexp.MustCompile(`(?s)does not serve security_sla_policy.*v0\.476\.3 or later`),
		}},
	})
}

func TestCompliancePeriodsAnswerTheirFreezeState(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Seed("compliance_periods", map[string]any{
		"id": "44444444-4444-4444-4444-444444444444", "name": "2026 H1", "framework": "soc2", "starts_on": "2026-01-01", "ends_on": "2026-06-30",
		"status": "open", "export_top_hash": nil, "state": "frozen", "frozen_at": "2026-07-01T07:10:00Z", "freeze_kind": "on_time",
		"catalogue_version": "2026.12", "fill_present_with_current": false,
	})
	const addr = "data.sreagent_compliance_periods.all"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_compliance_periods\" \"all\" {}\n",
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(addr, "items.0.state", "frozen"),
				resource.TestCheckResourceAttr(addr, "items.0.freeze_kind", "on_time"),
				resource.TestCheckResourceAttr(addr, "items.0.catalogue_version", "2026.12"),
				resource.TestCheckResourceAttr(addr, "items.0.fill_present_with_current", "false"),
			),
		}},
	})
}

func TestSlackAnswersItsDeliveryProblems(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Seed("slack", map[string]any{
		"id": "slack", "name": "Acme", "enabled": true,
		"delivery_problems": []any{map[string]any{"channel": "#alerts", "reason": "is_archived"}},
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_slack\" \"s\" {}\n",
			Check:  resource.TestMatchResourceAttr("data.sreagent_slack.s", "delivery_problems", regexpMust(`is_archived`)),
		}},
	})
}
