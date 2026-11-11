package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const outboundConfig = "sreagent_outbound_config.w"

func outboundConfigConfig(url, body string) string {
	return providerBlock(url) + fmt.Sprintf("resource \"sreagent_outbound_config\" \"w\" {\n  name = \"cards\"\n  provider_type = \"webhook\"\n  base_url_wo = \"https://hooks.example.com/cards\"\n  base_url_wo_version = 1\n%s\n}\n", body)
}

// card_events is sent when configured and read back; leaving it out sends
// nothing for the field, and a later change is sent on its own.
func TestOutboundConfigCardEvents(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: outboundConfigConfig(f.URL, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(outboundConfig, "card_events", "false"),
					func(*terraform.State) error {
						if v, sent := f.LastBody("POST", "outbound_configs")["card_events"]; sent {
							return fmt.Errorf("card_events was sent without being configured: %v", v)
						}
						return nil
					},
				),
			},
			{
				Config: outboundConfigConfig(f.URL, "  card_events = true"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(outboundConfig, "card_events", "true"),
					func(*terraform.State) error {
						if v := f.LastBody("PUT", "outbound_configs")["card_events"]; v != true {
							return fmt.Errorf("card_events sent as %v, want true", v)
						}
						return nil
					},
				),
			},
			{
				Config: outboundConfigConfig(f.URL, "  card_events = true\n  enabled = false"),
				Check: func(*terraform.State) error {
					if v, sent := f.LastBody("PUT", "outbound_configs")["card_events"]; sent {
						return fmt.Errorf("an unchanged card_events was resent: %v", v)
					}
					return nil
				},
			},
			{ResourceName: outboundConfig, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"base_url_wo", "base_url_wo_version"}, Config: outboundConfigConfig(f.URL, "  card_events = true\n  enabled = false")},
		},
	})
}

func TestOutboundConfigCardEventsCreatedTrue(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: outboundConfigConfig(f.URL, "  card_events = true"),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(outboundConfig, "card_events", "true"),
				func(*terraform.State) error {
					if v := f.LastBody("POST", "outbound_configs")["card_events"]; v != true {
						return fmt.Errorf("card_events sent as %v on create, want true", v)
					}
					return nil
				},
			),
		}},
	})
}
