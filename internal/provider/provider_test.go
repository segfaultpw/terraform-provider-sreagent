package provider

import (
	"fmt"
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

func regexpMust(s string) *regexp.Regexp { return regexp.MustCompile(s) }

// The pin's preflight read runs in Configure, before any resource is
// planned, so a key for another organization never reaches a write.
func TestOrganizationPinFailsBeforeAnyWrite(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      fmt.Sprintf("provider \"sreagent\" {\n  base_url = %q\n  api_key = \"sre_ak_unit\"\n  organization = \"other\"\n}\nresource \"sreagent_deploy_policy\" \"p\" {\n  service = \"web\"\n}\n", f.URL),
			ExpectError: regexp.MustCompile(`pinned to\s+"other"`),
		}},
	})
	if n := f.Calls("POST", "deploy_policies"); n != 0 {
		t.Fatalf("a pinned provider with another organization's key sent %d POSTs", n)
	}
	if f.Calls("GET", "organization_settings") == 0 {
		t.Fatal("the preflight read never ran")
	}
}

// A 409 names the row that already holds the key and the import line for it.
func TestAConflictNamesTheExistingRow(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	one := "resource \"sreagent_ticket_integration\" \"%s\" {\n  provider_type = \"jira\"\n  base_url = \"https://acme.atlassian.net\"\n}\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: providerBlock(f.URL) + fmt.Sprintf(one, "a")},
			{
				Config:      providerBlock(f.URL) + fmt.Sprintf(one, "a") + fmt.Sprintf(one, "b"),
				ExpectError: regexp.MustCompile(`(?s)id jira.*terraform import sreagent_ticket_integration\.<name> 'jira'`),
			},
		},
	})
}

// The platform trims and deduplicates list entries, so a padded or repeated
// one is refused at plan rather than failing as an inconsistent result.
func TestListEntriesMustBeTrimmedAndUnique(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(f.URL) + "resource \"sreagent_notification_settings\" \"n\" {\n  recipients = [\" a@x.com\", \"a@x.com\"]\n}\n",
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`(?s)whitespace`),
		}, {
			Config:      providerBlock(f.URL) + "resource \"sreagent_notification_settings\" \"n\" {\n  recipients = [\"a@x.com\", \"a@x.com\"]\n}\n",
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`duplicate values`),
		}},
	})
}
