package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

func TestListDataSourceReadsRowsWithoutSecrets(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + `
resource "sreagent_outbound_config" "pd" {
  name                   = "pd"
  provider_type          = "pagerduty"
  routing_key_wo         = "secret-routing"
  routing_key_wo_version = 1
}
data "sreagent_outbound_configs" "all" {
  depends_on = [sreagent_outbound_config.pd]
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.sreagent_outbound_configs.all", "items.#", "1"),
				resource.TestCheckResourceAttr("data.sreagent_outbound_configs.all", "items.0.name", "pd"),
				resource.TestCheckResourceAttr("data.sreagent_outbound_configs.all", "items.0.routing_key_set", "true"),
				resource.TestCheckNoResourceAttr("data.sreagent_outbound_configs.all", "items.0.routing_key"),
				resource.TestCheckResourceAttr("data.sreagent_outbound_configs.all", "truncated", "false"),
			),
		}},
	})
}

func TestSingletonDataSource(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_organization_settings\" \"o\" {}\n",
			Check:  resource.TestCheckResourceAttr("data.sreagent_organization_settings.o", "id", "organization_settings"),
		}},
	})
}

func TestTruncatedIsSurfaced(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.ForceTruncated("prompt_templates")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_prompt_templates\" \"p\" {}\n",
			Check:  resource.TestCheckResourceAttr("data.sreagent_prompt_templates.p", "truncated", "true"),
		}},
	})
}
