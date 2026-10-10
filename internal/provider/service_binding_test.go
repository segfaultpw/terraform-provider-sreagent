package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const bindingAddr = "sreagent_service_binding.b"

var bindingVersions = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)}

// bindingConfig is a service binding named checkout whose extra lines are body.
func bindingConfig(url, body string) string {
	return providerBlock(url) + "resource \"sreagent_service_binding\" \"b\" {\n  service = \"checkout\"\n" + body + "}\n"
}

// bodyHas checks whether the latest PUT body names key.
func bodyHas(f *fakefacade.Fake, key string, want bool) resource.TestCheckFunc {
	return func(*terraform.State) error {
		body := f.LastBody("PUT", "service_bindings")
		if body == nil {
			return fmt.Errorf("no PUT reached the platform")
		}
		if _, got := body[key]; got != want {
			return fmt.Errorf("the PUT body names %s: %v, want %v (body %v)", key, got, want, body)
		}
		return nil
	}
}

// The platform stores [] as null and answers null, so an empty list must apply
// and plan nothing afterwards, on create and on update.
func TestServiceBindingEmptyInlineFieldsApplyCleanly(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   bindingVersions,
		Steps: []resource.TestStep{
			{Config: bindingConfig(f.URL, "  log_inline_fields = []\n"), Check: resource.TestCheckResourceAttr(bindingAddr, "log_inline_fields.#", "0")},
			{Config: bindingConfig(f.URL, "  log_inline_fields = [\"trace_id\"]\n"), Check: resource.TestCheckResourceAttr(bindingAddr, "log_inline_fields.#", "1")},
			{Config: bindingConfig(f.URL, "  log_inline_fields = []\n"), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(bindingAddr, "log_inline_fields.#", "0"),
				func(*terraform.State) error {
					if v, ok := f.Row("service_bindings", "checkout")["log_inline_fields"]; ok && v != nil {
						return fmt.Errorf("the platform holds %v, want null", v)
					}
					return nil
				},
			)},
			{Config: bindingConfig(f.URL, ""), Check: resource.TestCheckNoResourceAttr(bindingAddr, "log_inline_fields.#")},
		},
	})
}

// An older platform refuses an argument its tool does not declare, so a binding
// that never sets log_inline_fields must not send it, null included.
func TestServiceBindingSendsOnlyWhatTheConfigurationSetsOrClears(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   bindingVersions,
		Steps: []resource.TestStep{
			{Config: bindingConfig(f.URL, "  log_groups = [\"/a\"]\n")},
			{
				Config: bindingConfig(f.URL, "  log_groups = [\"/a\", \"/b\"]\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					bodyHas(f, "log_groups", true),
					bodyHas(f, "log_inline_fields", false),
				),
			},
			{
				Config: bindingConfig(f.URL, "  log_groups = [\"/a\", \"/b\"]\n  log_inline_fields = [\"trace_id\"]\n"),
				Check:  bodyHas(f, "log_inline_fields", true),
			},
			{
				Config: bindingConfig(f.URL, "  log_groups = [\"/a\", \"/b\"]\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					bodyHas(f, "log_inline_fields", true),
					func(*terraform.State) error {
						if v := f.LastBody("PUT", "service_bindings")["log_inline_fields"]; v != nil {
							return fmt.Errorf("clearing must send null, sent %v", v)
						}
						return nil
					},
				),
			},
			{
				Config: bindingConfig(f.URL, "  log_groups = [\"/a\"]\n"),
				Check:  bodyHas(f, "log_inline_fields", false),
			},
		},
	})
}

// A configured [] against a platform that holds null changes nothing worth
// writing, but a later unrelated update still sends what the configuration says.
func TestServiceBindingEmptyListStaysQuietOnUnrelatedUpdates(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   bindingVersions,
		Steps: []resource.TestStep{
			{Config: bindingConfig(f.URL, "  log_groups = [\"/a\"]\n  log_inline_fields = []\n")},
			{Config: bindingConfig(f.URL, "  log_groups = [\"/a\", \"/b\"]\n  log_inline_fields = []\n"), Check: resource.TestCheckResourceAttr(bindingAddr, "log_inline_fields.#", "0")},
		},
	})
}

// The platform keeps a field name as written: a padded one is valid.
func TestServiceBindingKeepsAPaddedFieldNameAsWritten(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   bindingVersions,
		Steps: []resource.TestStep{{
			Config: bindingConfig(f.URL, "  log_inline_fields = [\" user.id\", \"a b\"]\n"),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(bindingAddr, "log_inline_fields.0", " user.id"),
				resource.TestCheckResourceAttr(bindingAddr, "log_inline_fields.1", "a b"),
			),
		}},
	})
}

// What the platform refuses and the configuration already says, refused at plan.
func TestServiceBindingRefusesAtPlanWhatThePlatformRefuses(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	for _, c := range []struct{ name, body, want string }{
		{"four fields", "  log_inline_fields = [\"a\", \"b\", \"c\", \"d\"]\n", `(?s)at most 3`},
		{"a duplicate", "  log_inline_fields = [\"a\", \"a\"]\n", `(?s)unique|duplicate`},
		{"a blank field", "  log_inline_fields = [\"a\", \"   \"]\n", `(?s)blank`},
		{"an empty field", "  log_inline_fields = [\"\"]\n", `(?s)blank|1 to 128`},
		{"a long field", "  log_inline_fields = [\"" + strings.Repeat("x", 129) + "\"]\n", `(?s)128 characters`},
		{"a control character", "  log_inline_fields = [\"a\\u0001b\"]\n", `(?s)control character`},
		{"an environment", "  environment = \"prod\"\n  log_inline_fields = [\"trace_id\"]\n", `(?s)Inline fields are set for the whole service`},
	} {
		t.Run(c.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				TerraformVersionChecks:   bindingVersions,
				Steps:                    []resource.TestStep{{Config: bindingConfig(f.URL, c.body), PlanOnly: true, ExpectError: regexp.MustCompile(c.want)}},
			})
		})
	}
	if f.Rows("service_bindings") != 0 {
		t.Fatal("a refused plan must write nothing")
	}
}

// The platform checks the environment only for a non-empty list, and an
// environment that is blank once trimmed is the whole service.
func TestServiceBindingAcceptsWhatThePlatformAccepts(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	for _, c := range []struct{ name, body string }{
		{"three fields", "  log_inline_fields = [\"a\", \"b\", \"c\"]\n"},
		{"an empty list beside an environment", "  environment = \"prod\"\n  log_inline_fields = []\n"},
		{"a blank environment", "  environment = \"  \"\n  log_inline_fields = [\"a\"]\n"},
		{"a 128 character field", "  log_inline_fields = [\"" + strings.Repeat("x", 128) + "\"]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				TerraformVersionChecks:   bindingVersions,
				Steps:                    []resource.TestStep{{Config: bindingConfig(f.URL, c.body), PlanOnly: true, ExpectNonEmptyPlan: true}},
			})
		})
	}
}
