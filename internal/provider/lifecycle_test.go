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
