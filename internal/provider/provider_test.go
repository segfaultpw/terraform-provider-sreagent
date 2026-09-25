package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

func TestReadOnlyKeyPlansButCannotApply(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.ReadOnly()
	cfg := providerBlock(f.URL) + "resource \"sreagent_deploy_policy\" \"p\" {\n  service = \"web\"\n}\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: cfg, ExpectError: regexp.MustCompile(`api:config_read, which can plan but not apply`)},
		},
	})
}

func TestProviderRefusesPlainHTTPToARemoteHost(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: `provider "sreagent" {
  base_url = "http://sreagent.app"
  api_key  = "sre_ak_x"
}
resource "sreagent_deploy_policy" "x" { service = "web" }`,
			ExpectError: regexp.MustCompile(`must use https`),
		}},
	})
}
