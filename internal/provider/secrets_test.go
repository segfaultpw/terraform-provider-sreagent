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
					if got := f.LastBody("PUT", "outbound_configs")["routing_key"]; got != sentinel {
						return fmt.Errorf("the new version must send the secret, sent %v", got)
					}
					return nil
				},
			},
			{
				// Another change at the same version leaves the secret out.
				Config: strings.Replace(cfg(2), `name = "pd"`, `name = "pd-2"`, 1),
				Check: func(*terraform.State) error {
					if _, sent := f.LastBody("PUT", "outbound_configs")["routing_key"]; sent {
						return fmt.Errorf("an unchanged version resent the secret: %v", f.LastBody("PUT", "outbound_configs"))
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

func TestAVersionBumpWithoutAValueIsRefused(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	base := providerBlock(f.URL) + "resource \"sreagent_outbound_config\" \"pd\" {\n  name = \"pd\"\n  provider_type = \"pagerduty\"\n%s}\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: fmt.Sprintf(base, "  routing_key_wo = \"one\"\n  routing_key_wo_version = 1\n")},
			{Config: fmt.Sprintf(base, "  routing_key_wo_version = 2\n"), ExpectError: regexpMust(`routing_key_wo_version changed but routing_key_wo is not set`)},
		},
	})
}

// The platform never answers a secret's value, only whether it holds one, so
// a secret lost on the platform while the configuration still sets it is the
// one drift a secret shows: it plans a re-send at the same version.
func TestALostSecretIsSentAgain(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_outbound_config\" \"pd\" {\n  name = \"pd\"\n  provider_type = \"pagerduty\"\n  routing_key_wo = %q\n  routing_key_wo_version = 1\n}\n", sentinel)
	var id string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg, Check: func(s *terraform.State) error {
				id = s.RootModule().Resources["sreagent_outbound_config.pd"].Primary.ID
				return nil
			}},
			{
				PreConfig:          func() { f.Mutate("outbound_configs", id, func(r map[string]any) { r["routing_key_set"] = false }) },
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: cfg, Check: func(*terraform.State) error {
				if n := f.Calls("PUT", "outbound_configs"); n != 1 {
					return fmt.Errorf("want exactly one PUT, got %d", n)
				}
				if got := f.LastBody("PUT", "outbound_configs")["routing_key"]; got != sentinel {
					return fmt.Errorf("the re-send must carry the secret, sent %v", got)
				}
				return nil
			}},
			{Config: cfg, PlanOnly: true},
		},
	})
}
