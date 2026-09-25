package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"sreagent": providerserver.NewProtocol6WithError(New("test")()),
}

// lifecycle is one resource's unit test against the fake facade.
type lifecycle struct {
	spec         engine.Spec
	create       string
	update       string
	after        map[string]string
	importIgnore []string
	// browserEdit changes a configuration field the way a person would in the browser.
	browserEdit func(map[string]any)
	// prelude is HCL for resources this one references.
	prelude string
}

func providerBlock(url string) string {
	return fmt.Sprintf(`provider "sreagent" {
  base_url = %q
  api_key  = "sre_ak_unit"
}
`, url)
}

func runLifecycle(t *testing.T, c lifecycle) {
	f := fakefacade.New(t, specs.All())
	addr := "sreagent_" + c.spec.TypeName + ".test"
	cfg := func(body string) string {
		return providerBlock(f.URL) + c.prelude + fmt.Sprintf("resource %q \"test\" {\n%s\n}\n", "sreagent_"+c.spec.TypeName, body)
	}
	checks := []resource.TestCheckFunc{}
	for k, v := range c.after {
		checks = append(checks, resource.TestCheckResourceAttr(addr, k, v))
	}
	idOf := func(s *terraform.State) string { return s.RootModule().Resources[addr].Primary.ID }
	var id string

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []resource.TestStep{
			{Config: cfg(c.create), Check: resource.ComposeAggregateTestCheckFunc(append(checks, func(s *terraform.State) error { id = idOf(s); return nil })...)},
			{Config: cfg(c.update)},
			{ResourceName: addr, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: c.importIgnore, Config: cfg(c.update)},
			{
				// A browser edit lands between refresh and write: the apply must refuse, not overwrite.
				PreConfig:   func() { f.MutateBeforeNextWrite(c.spec.Key, c.browserEdit) },
				Config:      cfg(c.create),
				ExpectError: regexp.MustCompile(`changed outside Terraform`),
			},
			{
				// Deleted in the browser: the next plan recreates it.
				PreConfig:          func() { f.Remove(c.spec.Key, id) },
				Config:             cfg(c.create),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestDeployPolicyLifecycle(t *testing.T) {
	runLifecycle(t, lifecycle{
		spec:        specs.DeployPolicy,
		create:      `service = "web"` + "\n" + `error_budget_threshold = 10`,
		update:      `service = "web"` + "\n" + `error_budget_threshold = 25` + "\n" + `incident_block_enabled = true`,
		after:       map[string]string{"service": "web", "error_budget_threshold": "10"},
		browserEdit: func(r map[string]any) { r["error_budget_threshold"] = 50 },
	})
}

func TestAlertRouteLifecycle(t *testing.T) {
	runLifecycle(t, lifecycle{
		spec:   specs.AlertRoute,
		create: `service = "Checkout"` + "\n" + `target = "#checkout-alerts"`,
		update: `service = "Checkout"` + "\n" + `target = "#checkout-incidents"`,
		after:  map[string]string{"service": "Checkout", "target": "#checkout-alerts"},
		// An import reads the stored, normalized "checkout"; the configured spelling is kept only once state exists.
		importIgnore: []string{"service"},
		browserEdit:  func(r map[string]any) { r["target"] = "#elsewhere" },
	})
}

func TestAlertRouteServiceChangeReplaces(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(svc string) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_alert_route\" \"r\" {\n  service = %q\n  target = \"#a\"\n}\n", svc)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg("checkout")},
			{
				Config: cfg("payments"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("sreagent_alert_route.r", plancheck.ResourceActionDestroyBeforeCreate),
				}},
			},
		},
	})
}

func TestAlertRouteClearsItsScheduleWhenRemovedFromConfig(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	base := providerBlock(f.URL) + "resource \"sreagent_alert_route\" \"r\" {\n  service = \"checkout\"\n  target = \"#a\"\n%s}\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: fmt.Sprintf(base, "  oncall_schedule_id = \"sched-1\"\n"), Check: resource.TestCheckResourceAttr("sreagent_alert_route.r", "oncall_schedule_id", "sched-1")},
			{Config: fmt.Sprintf(base, ""), Check: resource.TestCheckNoResourceAttr("sreagent_alert_route.r", "oncall_schedule_id")},
		},
	})
}

func TestOutboundConfigLifecycle(t *testing.T) {
	runLifecycle(t, lifecycle{
		spec:         specs.OutboundConfig,
		create:       "name = \"pd\"\nprovider_type = \"pagerduty\"\nrouting_key_wo = \"R0UTING-one\"\nrouting_key_wo_version = 1",
		update:       "name = \"pd-primary\"\nprovider_type = \"pagerduty\"\nrouting_key_wo = \"R0UTING-one\"\nrouting_key_wo_version = 1",
		after:        map[string]string{"routing_key_set": "true", "api_key_set": "false"},
		importIgnore: []string{"routing_key_wo_version"},
		browserEdit:  func(r map[string]any) { r["priority"] = 9 },
	})
}

func TestOutboundRuleLifecycle(t *testing.T) {
	runLifecycle(t, lifecycle{
		spec:        specs.OutboundRule,
		prelude:     "resource \"sreagent_outbound_config\" \"pd\" {\n  name = \"pd\"\n  provider_type = \"webhook\"\n  base_url = \"https://hooks.example.com/x\"\n}\n",
		create:      "name = \"sev1\"\noutbound_config_id = sreagent_outbound_config.pd.id\nmatch_severity = \"critical\"",
		update:      "name = \"sev1\"\noutbound_config_id = sreagent_outbound_config.pd.id",
		after:       map[string]string{"match_severity": "critical"},
		browserEdit: func(r map[string]any) { r["cooldown_minutes"] = 99 },
	})
}

func TestSyntheticCheckLifecycle(t *testing.T) {
	runLifecycle(t, lifecycle{
		spec:        specs.SyntheticCheck,
		create:      "name = \"home\"\ncheck_type = \"http\"\ntarget = \"https://example.com\"\ninterval_seconds = 60\nconfig = jsonencode({ method = \"GET\", tls_verify = true })",
		update:      "name = \"home\"\ncheck_type = \"http\"\ntarget = \"https://example.com\"\ninterval_seconds = 120\nconfig = jsonencode({ tls_verify = true, method = \"GET\" })",
		after:       map[string]string{"check_type": "http", "interval_seconds": "60"},
		browserEdit: func(r map[string]any) { r["target"] = "https://example.org" },
	})
}

func TestSyntheticCheckProbeResultsAreNotDrift(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := providerBlock(f.URL) + "resource \"sreagent_synthetic_check\" \"c\" {\n  name = \"home\"\n  check_type = \"http\"\n  target = \"https://example.com\"\n}\n"
	var id string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg, Check: func(s *terraform.State) error {
				id = s.RootModule().Resources["sreagent_synthetic_check.c"].Primary.ID
				return nil
			}},
			{
				PreConfig: func() {
					f.Mutate("synthetic_checks", id, func(r map[string]any) { r["last_status"] = "failure"; r["consecutive_failures"] = 3 })
				},
				Config:   cfg,
				PlanOnly: true,
			},
		},
	})
}

func TestSyntheticCheckConfigRefusesHeadersAndBody(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(f.URL) + "resource \"sreagent_synthetic_check\" \"c\" {\n  name = \"home\"\n  check_type = \"http\"\n  target = \"https://example.com\"\n  config = jsonencode({ method = \"GET\", headers = { Authorization = \"Bearer x\" } })\n}\n",
			ExpectError: regexp.MustCompile(`config.headers can carry a credential`),
		}},
	})
}

// config replaces the stored object as a whole, so an update that leaves it
// alone must not send it: resending it would drop headers set in the app.
func TestSyntheticCheckConfigIsSentOnlyWhenItChanges(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(extra string) string {
		return providerBlock(f.URL) + "resource \"sreagent_synthetic_check\" \"c\" {\n  name = \"home\"\n  check_type = \"http\"\n  target = \"https://example.com\"\n  config = jsonencode({ method = \"GET\" })\n" + extra + "}\n"
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg("")},
			{
				Config: cfg("  interval_seconds = 120\n"),
				Check: func(*terraform.State) error {
					if _, sent := f.LastBody("PUT", "synthetic_checks")["config"]; sent {
						return fmt.Errorf("an unchanged config was sent: %v", f.LastBody("PUT", "synthetic_checks"))
					}
					return nil
				},
			},
		},
	})
}

func TestSyntheticCheckConfigChangeIsRefusedWhileTheAppHoldsHeaders(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(method string) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_synthetic_check\" \"c\" {\n  name = \"home\"\n  check_type = \"http\"\n  target = \"https://example.com\"\n  config = jsonencode({ method = %q })\n}\n", method)
	}
	var id string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg("GET"), Check: func(s *terraform.State) error {
				id = s.RootModule().Resources["sreagent_synthetic_check.c"].Primary.ID
				return nil
			}},
			{
				// An Authorization header added in the app, which reads back only by name.
				PreConfig: func() {
					f.Mutate("synthetic_checks", id, func(r map[string]any) { r["config_header_names"] = []any{"Authorization"} })
				},
				Config:      cfg("HEAD"),
				ExpectError: regexp.MustCompile(`would silently drop them`),
			},
		},
	})
}

func TestBatchALifecycles(t *testing.T) {
	cases := []lifecycle{
		{spec: specs.DataSource, create: "name = \"prom\"\ntype = \"prometheus\"\nurl = \"https://prometheus.example.com\"", update: "name = \"prom\"\ntype = \"prometheus\"\nurl = \"https://prometheus-2.example.com\"\nenabled = false", after: map[string]string{"type": "prometheus"}, browserEdit: func(r map[string]any) { r["url"] = "https://other.example.com" }},
		{spec: specs.Connector, create: "name = \"box\"\nconnector_type = \"ssh\"\nconfig_wo = jsonencode({ host = \"10.0.0.1\" })\nconfig_wo_version = 1", update: "name = \"box-2\"\nconnector_type = \"ssh\"\nconfig_wo = jsonencode({ host = \"10.0.0.1\" })\nconfig_wo_version = 1", after: map[string]string{"config_set": "true"}, importIgnore: []string{"config_wo_version"}, browserEdit: func(r map[string]any) { r["name"] = "renamed" }},
		{spec: specs.SLI, create: "name = \"lat\"\nsli_type = \"latency\"\ndescription = \"p50 latency\"", update: "name = \"lat\"\nsli_type = \"latency\"\ndescription = \"p99 latency\"", after: map[string]string{"sli_type": "latency"}, browserEdit: func(r map[string]any) { r["name"] = "x" }},
		{spec: specs.SLO, prelude: "resource \"sreagent_sli\" \"s\" {\n  name = \"lat\"\n  sli_type = \"latency\"\n}\n", create: "name = \"o\"\nsli_id = sreagent_sli.s.id\ntarget = 99.9", update: "name = \"o\"\nsli_id = sreagent_sli.s.id\ntarget = 99.5", after: map[string]string{"target": "99.9"}, browserEdit: func(r map[string]any) { r["window_days"] = 7 }},
		{spec: specs.PromptTemplate, create: "name = \"p\"\nprompt_type = \"custom\"\ncontent = \"Summarize the alert.\"", update: "name = \"p\"\nprompt_type = \"custom\"\ncontent = \"Summarize the alert in two lines.\"", after: map[string]string{"prompt_type": "custom"}, browserEdit: func(r map[string]any) { r["content"] = "x" }},
		{spec: specs.Team, create: "name = \"platform\"", update: "name = \"platform-eng\"", after: map[string]string{"name": "platform"}, browserEdit: func(r map[string]any) { r["name"] = "other" }},
	}
	for _, c := range cases {
		t.Run(c.spec.TypeName, func(t *testing.T) { runLifecycle(t, c) })
	}
}

// No update tool: a change replaces the row, and there is no PUT for a 412 to refuse.
func TestImmutableCollectionsReplaceOnChange(t *testing.T) {
	for _, c := range []struct {
		typeName, before, after string
	}{
		{"alert_mute", "pattern = \"disk-full\"", "pattern = \"disk-full-2\""},
		{"certificate_monitor", "hostname = \"api.example.com\"", "hostname = \"api.example.com\"\nwarn_days_before = 14"},
	} {
		t.Run(c.typeName, func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			cfg := func(body string) string {
				return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_%s\" \"x\" {\n%s\n}\n", c.typeName, body)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{
					{Config: cfg(c.before)},
					{Config: cfg(c.after), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("sreagent_"+c.typeName+".x", plancheck.ResourceActionDestroyBeforeCreate),
					}}},
				},
			})
		})
	}
}

func TestDataSourceEnabledIsSentAfterCreate(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "resource \"sreagent_data_source\" \"d\" {\n  name = \"prom\"\n  type = \"prometheus\"\n  url = \"https://p.example.com\"\n  enabled = false\n}\n",
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("sreagent_data_source.d", "enabled", "false"),
				func(*terraform.State) error {
					if n := f.Calls("PUT", "data_sources"); n != 1 {
						return fmt.Errorf("enabled is not a create argument, so it needs one PUT after the POST; got %d", n)
					}
					return nil
				},
			),
		}},
	})
}

func TestBatchBLifecycles(t *testing.T) {
	cases := []lifecycle{
		{spec: specs.RepoSetting, create: "repo = \"acme/api\"\nbranch = \"main\"\npath_prefix = \"services\"", update: "repo = \"acme/api\"\nbranch = \"main\"\npath_prefix = \"services/api\"", after: map[string]string{"repo": "acme/api"}, browserEdit: func(r map[string]any) { r["service"] = "x" }},
		{spec: specs.ServiceBinding, create: "service = \"Checkout\"\nlog_groups = [\"/aws/lambda/checkout\"]", update: "service = \"Checkout\"\nlog_groups = [\"/aws/lambda/checkout\", \"/aws/ecs/checkout\"]", after: map[string]string{"id": "checkout"}, importIgnore: []string{"service"}, browserEdit: func(r map[string]any) { r["log_filter"] = "ERROR" }},
		{spec: specs.StatusPageComponent, create: "display_name = \"API\"", update: "display_name = \"Public API\"", after: map[string]string{"display_name": "API"}, browserEdit: func(r map[string]any) { r["display_name"] = "x" }},
		{spec: specs.AIProvider, create: "name = \"main\"\nprovider_type = \"anthropic\"\nmodel = \"claude-sonnet-5\"\napi_key_wo = \"sk-ant-unit\"\napi_key_wo_version = 1", update: "name = \"main\"\nprovider_type = \"anthropic\"\nmodel = \"claude-opus-5\"\napi_key_wo = \"sk-ant-unit\"\napi_key_wo_version = 1", after: map[string]string{"api_key_set": "true"}, importIgnore: []string{"api_key_wo_version"}, browserEdit: func(r map[string]any) { r["priority"] = 5 }},
		{spec: specs.TicketIntegration, create: "provider_type = \"jira\"\nbase_url = \"https://acme.atlassian.net\"\naccount_email = \"ops@example.com\"\napi_token_wo = \"tok-unit\"\napi_token_wo_version = 1", update: "provider_type = \"jira\"\nbase_url = \"https://acme.atlassian.net\"\naccount_email = \"sre@example.com\"\napi_token_wo = \"tok-unit\"\napi_token_wo_version = 1", after: map[string]string{"id": "jira", "api_token_set": "true"}, importIgnore: []string{"api_token_wo_version"}, browserEdit: func(r map[string]any) { r["project_key"] = "OPS" }},
	}
	for _, c := range cases {
		t.Run(c.spec.TypeName, func(t *testing.T) { runLifecycle(t, c) })
	}
}

func TestBatchCLifecycles(t *testing.T) {
	cases := []lifecycle{
		{spec: specs.TicketImportRule, create: "provider_type = \"jira\"\nenabled = false\nconfig = jsonencode({ project = \"OPS\" })", update: "provider_type = \"jira\"\nenabled = true\nconfig = jsonencode({ project = \"OPS\" })", after: map[string]string{"id": "jira"}, browserEdit: func(r map[string]any) { r["enabled"] = false }},
	}
	for _, c := range cases {
		t.Run(c.spec.TypeName, func(t *testing.T) { runLifecycle(t, c) })
	}
}

func TestNoUpdateResourcesReplace(t *testing.T) {
	for _, c := range []struct{ typeName, before, after, prelude string }{
		{"image_target", "image_ref = \"nginx:1.27\"", "image_ref = \"nginx:1.28\"", ""},
		{"team_member", "team_id = sreagent_team.t.id\nemail = \"a@example.com\"", "team_id = sreagent_team.t.id\nemail = \"b@example.com\"", "resource \"sreagent_team\" \"t\" {\n  name = \"platform\"\n}\n"},
	} {
		t.Run(c.typeName, func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			cfg := func(body string) string {
				return providerBlock(f.URL) + c.prelude + fmt.Sprintf("resource \"sreagent_%s\" \"x\" {\n%s\n}\n", c.typeName, body)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{
					{Config: cfg(c.before)},
					{Config: cfg(c.after), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("sreagent_"+c.typeName+".x", plancheck.ResourceActionDestroyBeforeCreate),
					}}},
				},
			})
		})
	}
}

func TestTeamMemberEmailCaseIsNotDrift(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := providerBlock(f.URL) + "resource \"sreagent_team\" \"t\" {\n  name = \"platform\"\n}\nresource \"sreagent_team_member\" \"m\" {\n  team_id = sreagent_team.t.id\n  email = \"Ana@Example.com\"\n}\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{Config: cfg, PlanOnly: true},
		},
	})
}

func TestDestroyingADiscoveredImageTargetWarns(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := providerBlock(f.URL) + "resource \"sreagent_image_target\" \"i\" {\n  image_ref = \"acme/api:1.4\"\n}\n"
	var id string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg, Check: func(s *terraform.State) error {
				id = s.RootModule().Resources["sreagent_image_target.i"].Primary.ID
				f.Mutate("image_targets", id, func(r map[string]any) { r["discovered"] = true })
				return nil
			}},
			{Config: cfg, PlanOnly: true, ExpectNonEmptyPlan: false},
			{Config: providerBlock(f.URL), Check: func(*terraform.State) error {
				if f.Row("image_targets", id) != nil {
					return fmt.Errorf("the target must be deleted")
				}
				return nil
			}},
		},
	})
}

// runSingleton is runLifecycle without the delete-drift step: a singleton
// cannot be deleted, only forgotten.
func runSingleton(t *testing.T, spec engine.Spec, create, update string, browserEdit func(map[string]any)) {
	t.Run(spec.TypeName, func(t *testing.T) {
		f := fakefacade.New(t, specs.All())
		addr := "sreagent_" + spec.TypeName + ".test"
		cfg := func(body string) string {
			return providerBlock(f.URL) + fmt.Sprintf("resource %q \"test\" {\n%s\n}\n", "sreagent_"+spec.TypeName, body)
		}
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: factories,
			TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
			Steps: []resource.TestStep{
				{Config: cfg(create)},
				{Config: cfg(update)},
				{ResourceName: addr, ImportState: true, ImportStateId: spec.Key, ImportStateVerify: true, ImportStateVerifyIgnore: secretVersions(spec), Config: cfg(update)},
				{
					PreConfig:   func() { f.MutateBeforeNextWrite(spec.Key, browserEdit) },
					Config:      cfg(create),
					ExpectError: regexp.MustCompile(`changed outside Terraform`),
				},
			},
		})
	})
}

// secretVersions is every <secret>_wo_version of spec, which an import cannot know.
func secretVersions(spec engine.Spec) []string {
	out := []string{}
	for _, a := range spec.Attrs {
		if a.Secret {
			out = append(out, a.Attribute()+"_wo_version")
		}
	}
	return out
}

func TestSingletonLifecycles(t *testing.T) {
	runSingleton(t, specs.OrganizationSettings, "alert_storm_threshold = 20", "alert_storm_threshold = 30\ntimezone = \"America/Sao_Paulo\"", func(r map[string]any) { r["alert_storm_threshold"] = 99 })
	runSingleton(t, specs.NotificationSettings, "notify_slo = true", "notify_slo = false", func(r map[string]any) { r["notify_slo"] = true })
	runSingleton(t, specs.Slack, "name = \"ws\"\nbot_token_wo = \"xoxb-unit\"\nbot_token_wo_version = 1", "name = \"ws-2\"\ndefault_channel_critical = \"#pager\"\nbot_token_wo = \"xoxb-unit\"\nbot_token_wo_version = 1", func(r map[string]any) { r["name"] = "other" })
	runSingleton(t, specs.ChangeNotifications, "enabled = true\nchannel = \"#changes\"", "enabled = true\nchannel = \"#deploys\"", func(r map[string]any) { r["channel"] = "#x" })
	runSingleton(t, specs.AISettings, "anonymization_enabled = true", "anonymization_enabled = false", func(r map[string]any) { r["anonymization_enabled"] = true })
	runSingleton(t, specs.OverseerSettings, "enabled = true\ndigest_enabled = false", "enabled = true\ndigest_enabled = true", func(r map[string]any) { r["cadence"] = "weekly" })
	runSingleton(t, specs.GitHubSettings, "draft_prs = true", "draft_prs = false", func(r map[string]any) { r["draft_prs"] = true })
	runSingleton(t, specs.StatusPageSettings, "enabled = true\nshow_history_days = 14", "enabled = true\nshow_history_days = 30", func(r map[string]any) { r["title"] = "changed" })
}
