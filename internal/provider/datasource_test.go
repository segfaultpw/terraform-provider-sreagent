package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
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

// A singleton an organization has no row of reads as configured = false with
// null attributes, so HCL can branch on it instead of failing the plan.
func TestASingletonWithNoRowReadsAsNotConfigured(t *testing.T) {
	for key, field := range map[string]string{"slack": "enabled", "change_notifications": "channel", "github_settings": "draft_prs"} {
		t.Run(key, func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			// github_settings answers a row on a fresh organization; one that
			// only inherits its parent's reads as having none of its own.
			f.Remove(key, key)
			addr := "data.sreagent_" + key + ".x"
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config: providerBlock(f.URL) + "data \"sreagent_" + key + "\" \"x\" {}\n",
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr(addr, "configured", "false"),
						resource.TestCheckResourceAttr(addr, "id", key),
						resource.TestCheckNoResourceAttr(addr, field),
					),
				}},
			})
		})
	}
}

func TestASingletonWithARowReadsAsConfigured(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_organization_settings\" \"x\" {}\n",
			Check:  resource.TestCheckResourceAttr("data.sreagent_organization_settings.x", "configured", "true"),
		}},
	})
}

// The request signing data source answers key ids, states and the destination list as JSON, and
// has no attribute that could hold a key: the key is revealed on the platform's Settings page only.
func TestRequestSigningDataSourceAnswersKidsAndNeverAKey(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("request_signing", "request_signing", func(row map[string]any) {
		row["keys"] = []any{map[string]any{"kid": "k_abc", "state": "signing", "activates_at": "2026-10-10T00:00:00Z", "verify_until": nil}}
		row["destinations"] = []any{map[string]any{"host": "metrics.acme.example", "outcome": "signed", "sources": []any{"data source Prom"}}}
		row["remote_locations_pending"] = true
		row["header"] = "SRE-Agent-Signature: v=1,kid=...,t=...,n=...,b=...,s=..."
		row["docs_url"] = "https://sreagent.app/docs/request-signing"
	})
	addr := "data.sreagent_request_signing.x"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_request_signing\" \"x\" {}\n" +
				"output \"kid\" { value = jsondecode(data.sreagent_request_signing.x.keys)[0].kid }\n",
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(addr, "id", "request_signing"),
				resource.TestCheckResourceAttr(addr, "configured", "true"),
				resource.TestCheckResourceAttr(addr, "remote_locations_pending", "true"),
				resource.TestCheckResourceAttr(addr, "header", "SRE-Agent-Signature: v=1,kid=...,t=...,n=...,b=...,s=..."),
				resource.TestCheckResourceAttr(addr, "docs_url", "https://sreagent.app/docs/request-signing"),
				resource.TestCheckResourceAttrSet(addr, "keys"),
				resource.TestCheckResourceAttrSet(addr, "destinations"),
				resource.TestCheckOutput("kid", "k_abc"),
				resource.TestCheckNoResourceAttr(addr, "key"),
				resource.TestCheckNoResourceAttr(addr, "secret"),
			),
		}},
	})
}

// Every organization has request signing, so a 404 means the platform is older than the
// resource. That is an error naming the release needed, never configured = false with null
// attributes that a jsondecode further down would trip over.
func TestRequestSigningOnAPlatformWithoutTheResourceIsAnError(t *testing.T) {
	cases := map[string]struct {
		build func(t *testing.T) *fakefacade.Fake
		// said is the platform's own sentence, which the error keeps.
		said string
	}{
		"the row is missing": {func(t *testing.T) *fakefacade.Fake {
			f := fakefacade.New(t, specs.All())
			f.Remove("request_signing", "request_signing")
			return f
		}, `No\s+row\s+found`},
		"the resource is unknown": {func(t *testing.T) *fakefacade.Fake {
			var older []engine.Spec
			for _, s := range specs.All() {
				if s.Key != "request_signing" {
					older = append(older, s)
				}
			}
			return fakefacade.New(t, older)
		}, `No\s+such\s+configuration\s+resource:\s+request_signing`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := c.build(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config:      providerBlock(f.URL) + "data \"sreagent_request_signing\" \"x\" {}\n",
					ExpectError: regexp.MustCompile(`(?s)platform does not serve request_signing.*v0\.395\.0 or later.*` + c.said),
				}},
			})
		})
	}
}

func TestRequestSigningSpecHasNoAttributeThatCouldHoldAKey(t *testing.T) {
	for _, a := range specs.RequestSigning.Attrs {
		if a.Secret || !a.Computed {
			t.Fatalf("%s must be a plain computed attribute", a.Name)
		}
		for _, bad := range []string{"key", "secret", "token"} {
			if a.Name == bad {
				t.Fatalf("attribute %s could hold key material", a.Name)
			}
		}
	}
}
