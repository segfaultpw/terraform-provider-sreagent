package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const sentinel = "R0UTING-SENTINEL-7f3a"

func TestSecretsNeverReachStateOrPlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(version int) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_outbound_config\" \"pd\" {\n  name = \"pd\"\n  provider_type = \"pagerduty\"\n  routing_key_wo = %q\n  routing_key_wo_version = %d\n}\n", sentinel, version)
	}
	noSentinel := func(s *terraform.State) error {
		for _, r := range s.RootModule().Resources {
			for k, v := range r.Primary.Attributes {
				if strings.Contains(v, sentinel) {
					return fmt.Errorf("state attribute %s holds the secret", k)
				}
			}
		}
		return nil
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg(1), Check: resource.ComposeAggregateTestCheckFunc(noSentinel, resource.TestCheckResourceAttr("sreagent_outbound_config.pd", "routing_key_set", "true"))},
			{
				// Same version: the secret is not sent again.
				Config: cfg(1),
				Check: func(*terraform.State) error {
					if n := f.Calls("PUT", "outbound_configs"); n != 0 {
						return fmt.Errorf("an unchanged version sent %d PUTs", n)
					}
					return nil
				},
			},
			{
				// A new version sends it once.
				Config: cfg(2),
				Check: func(*terraform.State) error {
					if n := f.Calls("PUT", "outbound_configs"); n != 1 {
						return fmt.Errorf("want one PUT, got %d", n)
					}
					return nil
				},
			},
		},
	})
}

func TestAWriteOnlyValueWithoutAVersionIsRefused(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(f.URL) + "resource \"sreagent_outbound_config\" \"pd\" {\n  name = \"pd\"\n  provider_type = \"pagerduty\"\n  routing_key_wo = \"x\"\n}\n",
			ExpectError: regexpMust(`routing_key_wo_version`),
		}},
	})
}
