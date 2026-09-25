package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

type accCase struct {
	name, typeName, key, create, update, prelude string
	importIgnore                                 []string
}

func accClient(t *testing.T) *client.Client {
	t.Helper()
	c, err := client.New(client.Config{BaseURL: os.Getenv("SREAGENT_BASE_URL"), APIKey: os.Getenv("SREAGENT_API_KEY"), UserAgent: "acceptance", MaxRetries: 2})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func accProvider() string {
	return fmt.Sprintf("provider \"sreagent\" {\n  organization = %q\n}\n", os.Getenv("SREAGENT_ORGANIZATION"))
}

func runAcc(t *testing.T, c accCase) {
	addr := "sreagent_" + c.typeName + ".test"
	cfg := func(body string) string {
		return accProvider() + c.prelude + fmt.Sprintf("resource %q \"test\" {\n%s\n}\n", "sreagent_"+c.typeName, body)
	}
	var id string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []resource.TestStep{
			{Config: cfg(c.create), Check: func(s *terraform.State) error { id = s.RootModule().Resources[addr].Primary.ID; return nil }},
			{Config: cfg(c.update)},
			{ResourceName: addr, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: c.importIgnore, Config: cfg(c.update)},
			{
				PreConfig: func() {
					if _, err := accClient(t).Do(context.Background(), client.Request{Method: http.MethodDelete, Path: c.key + "/" + id}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             cfg(c.update),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccResources(t *testing.T) {
	for _, c := range []accCase{
		{name: "deploy_policy", typeName: "deploy_policy", key: "deploy_policies", create: "service = \"acc-web\"\nerror_budget_threshold = 10", update: "service = \"acc-web\"\nerror_budget_threshold = 25"},
		{name: "alert_route", typeName: "alert_route", key: "alert_routes", create: "service = \"Acc-Checkout\"\ntarget = \"#acc-alerts\"", update: "service = \"Acc-Checkout\"\ntarget = \"#acc-incidents\"", importIgnore: []string{"service"}},
		{name: "outbound_config", typeName: "outbound_config", key: "outbound_configs", create: "name = \"acc-pd\"\nprovider_type = \"pagerduty\"\nrouting_key_wo = \"R0UTING0acc0000000000000000000\"\nrouting_key_wo_version = 1", update: "name = \"acc-pd-2\"\nprovider_type = \"pagerduty\"\nrouting_key_wo = \"R0UTING0acc0000000000000000000\"\nrouting_key_wo_version = 1", importIgnore: []string{"routing_key_wo_version"}},
		{name: "outbound_rule", typeName: "outbound_rule", key: "outbound_rules", prelude: "resource \"sreagent_outbound_config\" \"t\" {\n  name = \"acc-hook\"\n  provider_type = \"webhook\"\n  base_url = \"https://hooks.example.com/acc\"\n}\n", create: "name = \"acc-sev1\"\noutbound_config_id = sreagent_outbound_config.t.id\nmatch_severity = \"critical\"", update: "name = \"acc-sev1\"\noutbound_config_id = sreagent_outbound_config.t.id"},
		{name: "synthetic_check", typeName: "synthetic_check", key: "synthetic_checks", create: "name = \"acc-home\"\ncheck_type = \"http\"\ntarget = \"https://example.com\"", update: "name = \"acc-home\"\ncheck_type = \"http\"\ntarget = \"https://example.com\"\ninterval_seconds = 120"},
		{name: "data_source", typeName: "data_source", key: "data_sources", create: "name = \"acc-prom\"\ntype = \"prometheus\"\nurl = \"https://prometheus.example.com\"", update: "name = \"acc-prom\"\ntype = \"prometheus\"\nurl = \"https://prometheus.example.com\"\nenabled = false"},
		{name: "connector", typeName: "connector", key: "connectors", create: "name = \"acc-box\"\nconnector_type = \"ssh\"\nconfig_wo = jsonencode({ host = \"127.0.0.1\" })\nconfig_wo_version = 1", update: "name = \"acc-box-2\"\nconnector_type = \"ssh\"\nconfig_wo = jsonencode({ host = \"127.0.0.1\" })\nconfig_wo_version = 1", importIgnore: []string{"config_wo_version"}},
		{name: "sli", typeName: "sli", key: "slis", create: "name = \"acc-lat\"\nsli_type = \"latency\"", update: "name = \"acc-lat\"\nsli_type = \"latency\"\ndescription = \"p99 latency\""},
		{name: "slo", typeName: "slo", key: "slos", prelude: "resource \"sreagent_sli\" \"s\" {\n  name = \"acc-slo-sli\"\n  sli_type = \"latency\"\n}\n", create: "name = \"acc-o\"\nsli_id = sreagent_sli.s.id\ntarget = 99.9", update: "name = \"acc-o\"\nsli_id = sreagent_sli.s.id\ntarget = 99.5"},
		{name: "alert_mute", typeName: "alert_mute", key: "alert_mutes", create: "pattern = \"acc-disk-full\"", update: "pattern = \"acc-disk-full\""},
		{name: "certificate_monitor", typeName: "certificate_monitor", key: "certificate_monitors", create: "hostname = \"example.com\"", update: "hostname = \"example.com\""},
		{name: "prompt_template", typeName: "prompt_template", key: "prompt_templates", create: "name = \"acc-p\"\nprompt_type = \"custom\"\ncontent = \"Summarize the alert.\"", update: "name = \"acc-p\"\nprompt_type = \"custom\"\ncontent = \"Summarize the alert in two lines.\""},
		{name: "team", typeName: "team", key: "teams", create: "name = \"acc-platform\"", update: "name = \"acc-platform-eng\""},
		{name: "service_binding", typeName: "service_binding", key: "service_bindings", create: "service = \"Acc-Checkout\"\nlog_groups = [\"/aws/lambda/acc\"]", update: "service = \"Acc-Checkout\"\nlog_groups = [\"/aws/lambda/acc\", \"/aws/ecs/acc\"]", importIgnore: []string{"service"}},
		{name: "status_page_component", typeName: "status_page_component", key: "status_page_components", create: "display_name = \"Acc API\"", update: "display_name = \"Acc Public API\""},
		{name: "ai_provider", typeName: "ai_provider", key: "ai_providers", create: "name = \"acc-main\"\nprovider_type = \"anthropic\"\nmodel = \"claude-sonnet-5\"\napi_key_wo = \"sk-ant-acc-0000\"\napi_key_wo_version = 1", update: "name = \"acc-main\"\nprovider_type = \"anthropic\"\nmodel = \"claude-opus-5\"\napi_key_wo = \"sk-ant-acc-0000\"\napi_key_wo_version = 1", importIgnore: []string{"api_key_wo_version"}},
		{name: "ticket_integration", typeName: "ticket_integration", key: "ticket_integrations", create: "provider_type = \"jira\"\nbase_url = \"https://acme.atlassian.net\"\naccount_email = \"ops@example.com\"\napi_token_wo = \"tok-acc-0000\"\napi_token_wo_version = 1", update: "provider_type = \"jira\"\nbase_url = \"https://acme.atlassian.net\"\naccount_email = \"sre@example.com\"\napi_token_wo = \"tok-acc-0000\"\napi_token_wo_version = 1", importIgnore: []string{"api_token_wo_version"}},
	} {
		t.Run(c.name, func(t *testing.T) { runAcc(t, c) })
	}
}

// cli runs Terraform against a locally built provider through dev_overrides.
type cli struct {
	t   *testing.T
	dir string
	env []string
}

func newCLI(t *testing.T) *cli {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "terraform-provider-sreagent"), "../..") //nolint:gosec // a fixed build of this repository into the test's temp dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the provider: %s", out)
	}
	rc := fmt.Sprintf("provider_installation {\n  dev_overrides {\n    \"segfaultpw/sreagent\" = %q\n  }\n  direct {}\n}\n", bin)
	if err := os.WriteFile(filepath.Join(dir, "dev.tfrc"), []byte(rc), 0o600); err != nil {
		t.Fatal(err)
	}
	return &cli{t: t, dir: dir, env: append(os.Environ(), "TF_CLI_CONFIG_FILE="+filepath.Join(dir, "dev.tfrc"), "TF_IN_AUTOMATION=1")}
}

func (c *cli) write(body string) {
	c.t.Helper()
	hcl := "terraform {\n  required_providers {\n    sreagent = { source = \"segfaultpw/sreagent\" }\n  }\n}\n" + accProvider() + body
	if err := os.WriteFile(filepath.Join(c.dir, "main.tf"), []byte(hcl), 0o600); err != nil {
		c.t.Fatal(err)
	}
}

func (c *cli) run(args ...string) (string, error) {
	cmd := exec.Command("terraform", append(args, "-no-color")...) //nolint:gosec // the tests name the subcommands
	cmd.Dir = c.dir
	cmd.Env = c.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestAccStalePlanIsRefused(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance only")
	}
	tf := newCLI(t)
	t.Cleanup(func() { _, _ = tf.run("destroy", "-auto-approve") })

	tf.write("resource \"sreagent_deploy_policy\" \"p\" {\n  service = \"acc-stale-plan\"\n  error_budget_threshold = 10\n}\n")
	if out, err := tf.run("apply", "-auto-approve"); err != nil {
		t.Fatal(out)
	}
	shown, err := tf.run("state", "show", "sreagent_deploy_policy.p")
	if err != nil {
		t.Fatal(shown)
	}
	id := regexp.MustCompile(`(?m)^\s*id\s+=\s+"([^"]+)"`).FindStringSubmatch(shown)[1]

	tf.write("resource \"sreagent_deploy_policy\" \"p\" {\n  service = \"acc-stale-plan\"\n  error_budget_threshold = 20\n}\n")
	if out, err := tf.run("plan", "-out=stale.tfplan"); err != nil {
		t.Fatal(out)
	}
	// The platform's default is true, so false is a change that moves the ETag.
	if _, err := accClient(t).Do(context.Background(), client.Request{Method: http.MethodPut, Path: "deploy_policies/" + id, Body: map[string]any{"incident_block_enabled": false}}); err != nil {
		t.Fatal(err)
	}
	out, err := tf.run("apply", "stale.tfplan")
	if err == nil || !regexp.MustCompile(`changed outside Terraform`).MatchString(out) {
		t.Fatalf("a saved plan applied after a browser edit must be refused, got: %s", out)
	}
}

// A CI job holding only the api:config_read key can plan against real rows and cannot apply.
func TestAccReadOnlyKeyPlansButCannotApply(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance only")
	}
	admin := newCLI(t)
	t.Cleanup(func() { _, _ = admin.run("destroy", "-auto-approve") })
	body := "resource \"sreagent_deploy_policy\" \"p\" {\n  service = \"acc-read-only\"\n  error_budget_threshold = 10\n}\n"
	admin.write(body)
	if out, err := admin.run("apply", "-auto-approve"); err != nil {
		t.Fatal(out)
	}

	reader := &cli{t: t, dir: admin.dir, env: append(append([]string{}, admin.env...), "SREAGENT_API_KEY="+os.Getenv("SREAGENT_READ_API_KEY"))}
	if out, err := reader.run("plan", "-detailed-exitcode"); err != nil {
		t.Fatalf("a read-only key must plan cleanly against unchanged rows: %s", out)
	}
	admin.write(strings.Replace(body, "10", "20", 1))
	out, err := reader.run("apply", "-auto-approve")
	if err == nil || !regexp.MustCompile(`can plan but not apply`).MatchString(out) {
		t.Fatalf("a read-only key must be refused on apply with the read-only diagnostic, got: %s", out)
	}
}
