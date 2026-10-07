package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

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

const hookURL = "https://hooks.example.com/T0/B0/tokened-path"

// A webhook URL carries its own token, so it is write-only. Removing its value
// and its version clears it on the platform, the way unsetting the old plain
// argument did; removing the api key's pair keeps the key.
func TestRemovingTheURLsValueAndVersionClearsItButTheKeyIsKept(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(extra string) string {
		return providerBlock(f.URL) + "resource \"sreagent_outbound_config\" \"hook\" {\n  name = \"hook\"\n  provider_type = \"webhook\"\n" + extra + "}\n"
	}
	both := fmt.Sprintf("  base_url_wo = %q\n  base_url_wo_version = 1\n  api_key_wo = \"whsec-unit\"\n  api_key_wo_version = 1\n", hookURL)
	noURL := "  api_key_wo = \"whsec-unit\"\n  api_key_wo_version = 1\n"
	neither := ""
	noPut := func(*terraform.State) error {
		if n := f.Calls("PUT", "outbound_configs"); n != 0 {
			return fmt.Errorf("want no PUT, got %d", n)
		}
		return nil
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg(both), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("sreagent_outbound_config.hook", "base_url_set", "true"),
				resource.TestCheckResourceAttr("sreagent_outbound_config.hook", "base_url_host", "https://hooks.example.com"),
				resource.TestCheckResourceAttr("sreagent_outbound_config.hook", "api_key_set", "true"),
				func(s *terraform.State) error {
					for k, v := range s.RootModule().Resources["sreagent_outbound_config.hook"].Primary.Attributes {
						if strings.Contains(v, "tokened-path") {
							return fmt.Errorf("state attribute %s holds the URL", k)
						}
					}
					return nil
				},
				noPut,
			)},
			{
				// The value left out with the version unchanged keeps the URL.
				Config: cfg("  base_url_wo_version = 1\n  api_key_wo = \"whsec-unit\"\n  api_key_wo_version = 1\n"),
				Check:  noPut,
			},
			{
				// Both removed: one PUT with base_url null, and nothing for the key.
				Config: cfg(noURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("sreagent_outbound_config.hook", "base_url_set", "false"),
					resource.TestCheckNoResourceAttr("sreagent_outbound_config.hook", "base_url_host"),
					resource.TestCheckResourceAttr("sreagent_outbound_config.hook", "api_key_set", "true"),
					func(*terraform.State) error {
						if n := f.Calls("PUT", "outbound_configs"); n != 1 {
							return fmt.Errorf("want one PUT, got %d", n)
						}
						body := f.LastBody("PUT", "outbound_configs")
						if v, present := body["base_url"]; !present || v != nil {
							return fmt.Errorf("want base_url: null, sent %v", body)
						}
						if _, present := body["api_key"]; present {
							return fmt.Errorf("the key must not be sent: %v", body)
						}
						return nil
					},
				),
			},
			{Config: cfg(noURL), PlanOnly: true},
			{
				// The key's pair removed as well keeps the key: only the required
				// fields go out, never the key and never a second clear.
				Config: cfg(neither),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("sreagent_outbound_config.hook", "api_key_set", "true"),
					func(*terraform.State) error {
						body := f.LastBody("PUT", "outbound_configs")
						for _, k := range []string{"api_key", "base_url"} {
							if _, present := body[k]; present {
								return fmt.Errorf("removing the key's pair sent %s: %v", k, body)
							}
						}
						return nil
					},
				),
			},
			{Config: cfg(neither), PlanOnly: true},
		},
	})
}

// An adopted row has no recorded version, so a configuration that never names
// the URL leaves the stored one alone.
func TestAnAdoptedURLIsNeverClearedByLeavingItOut(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	id := seedOne(t, f.URL, "outbound_configs", map[string]any{"name": "hook", "provider_type": "webhook", "base_url": hookURL})
	config := providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_outbound_config\" \"hook\" {\n  name = \"hook\"\n  provider_type = \"webhook\"\n}\n\nimport {\n  to = sreagent_outbound_config.hook\n  id = %q\n}\n", id)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []resource.TestStep{{
			Config: config,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("sreagent_outbound_config.hook", "base_url_set", "true"),
				func(*terraform.State) error {
					if n := f.Calls("PUT", "outbound_configs"); n != 0 {
						return fmt.Errorf("an adopted URL was written over: %d PUTs", n)
					}
					return nil
				},
			),
		}},
	})
}
