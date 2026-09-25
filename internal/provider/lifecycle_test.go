package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
