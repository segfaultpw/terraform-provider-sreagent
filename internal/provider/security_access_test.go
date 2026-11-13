package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// securityResource is one of the five managed security resources, with a
// configuration that creates it and one that changes it.
type securityResource struct {
	spec   engine.Spec
	name   string
	create string
	change string
	// prepare readies the fake the way the organization would be (accepted classes).
	prepare func(*fakefacade.Fake)
}

func (r securityResource) addr() string { return "sreagent_" + r.spec.TypeName + "." + r.name }

func (r securityResource) config(f *fakefacade.Fake, body string) string {
	if r.spec.Key == "secret_allow_rules" {
		return providerBlock(f.URL) + fmt.Sprintf("resource %q %q {\n  path_glob = \"vendor/**\"\n%s}\n", "sreagent_"+r.spec.TypeName, r.name, body)
	}
	return providerBlock(f.URL) + fmt.Sprintf("resource %q %q {\n%s}\n", "sreagent_"+r.spec.TypeName, r.name, body)
}

func securityResources() []securityResource {
	return []securityResource{
		{spec: specs.SecuritySLAPolicy, name: "p", create: "  critical_days = 5\n", change: "  critical_days = 6\n"},
		{spec: specs.SecurityDigestSettings, name: "d", create: "  enabled = false\n", change: "  hour = 10\n"},
		{spec: specs.SecurityCodeClasses, name: "c", create: "  classes = []\n", change: "  classes = [\"other:a\"]\n", prepare: func(f *fakefacade.Fake) { f.SetAcceptedClasses("other:a") }},
		{spec: specs.SecurityNotificationRules, name: "r", create: "  name = \"rule\"\n", change: "  name = \"renamed\"\n"},
		{spec: specs.SecretAllowRules, name: "a", create: "  reason = \"Vendored test fixtures\"\n", change: "  reason = \"Vendored test fixtures, again\"\n"},
	}
}

func TestSecurityResourcesReadOnlyKeyPlansButDoesNotApply(t *testing.T) {
	for _, r := range securityResources() {
		t.Run(r.spec.TypeName, func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			if r.prepare != nil {
				r.prepare(f)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{
					{Config: r.config(f, r.create)},
					// An api:config_read key reads, so the plan works.
					{PreConfig: f.ReadOnly, Config: r.config(f, r.create), PlanOnly: true},
					{Config: r.config(f, r.change), ExpectError: wrapped("api:config_read, which can plan but not apply")},
					{PreConfig: f.AllowWrites, Config: r.config(f, r.create)},
				},
			})
		})
	}
}

// A plan that lost the infrastructure suite fails with the platform's sentence, the feature and the
// exits that work, for every verb: it must never read as a row deleted outside Terraform, which
// would plan a create.
func TestSecurityResourcesNameThePlanWhenTheSuiteIsGone(t *testing.T) {
	for _, r := range securityResources() {
		t.Run(r.spec.TypeName, func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			if r.prepare != nil {
				r.prepare(f)
			}
			typeName := "sreagent_" + r.spec.TypeName
			exits := wrapped("terraform state rm "+typeName+".<name>", "removed block whose lifecycle sets destroy = false")
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{
					{Config: r.config(f, r.create)},
					{
						PreConfig: func() { f.RefusePlan(r.spec.Key, infrastructureRefusal) },
						Config:    r.config(f, r.create),
						PlanOnly:  true,
						ExpectError: wrapped("Could not read "+typeName, "The platform answered 403", infrastructureRefusal,
							"needs the infrastructure suite in the organization's plan", "Nothing was changed."),
					},
					{Config: r.config(f, r.change), ExpectError: exits},
					{
						// The exit the text names works while every read answers 403.
						Config: providerBlock(f.URL) + fmt.Sprintf("removed {\n  from = %s\n\n  lifecycle {\n    destroy = false\n  }\n}\n", r.addr()),
						Check: func(s *terraform.State) error {
							if n := len(s.RootModule().Resources); n != 0 {
								return fmt.Errorf("the resource must be forgotten, state holds %d", n)
							}
							return nil
						},
					},
				},
			})
		})
	}
}

func TestSecurityResourcesCreateRefusedForThePlanNamesTheFeature(t *testing.T) {
	for _, r := range securityResources() {
		t.Run(r.spec.TypeName, func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			before := f.Rows(r.spec.Key)
			f.RefusePlan(r.spec.Key, infrastructureRefusal)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config:      r.config(f, r.create),
					ExpectError: wrapped("Could not create sreagent_"+r.spec.TypeName, infrastructureRefusal, "needs the infrastructure suite"),
				}},
			})
			if f.Rows(r.spec.Key) != before {
				t.Errorf("a refused create writes nothing")
			}
		})
	}
}

// None of the five holds a secret: every attribute is plain state, nothing is sensitive and
// nothing is write-only.
func TestNoSecurityResourceHasASensitiveOrWriteOnlyAttribute(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	for _, r := range securityResources() {
		d := newDirect(t, r.spec, f)
		for name, a := range d.schema.Schema.Attributes {
			if a.IsSensitive() || a.IsWriteOnly() {
				t.Errorf("sreagent_%s.%s is sensitive or write-only; the security resources hold no secret", r.spec.TypeName, name)
			}
		}
	}
}
