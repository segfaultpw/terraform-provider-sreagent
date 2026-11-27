package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// An unordered list makes a diff whose other attributes match state a spelling-only one, and
// the plan then keeps every computed value from state. A secret whose version changed is not
// spelling, whatever the secret's kind: the new value has to be sent.
func scratchSecretSpec(kind engine.Kind) engine.Spec {
	return engine.Spec{
		Key: "scratch_secret", TypeName: "scratch_secret", ListName: "scratch_secret", Shape: engine.Generated, Lifecycle: true,
		Description: "A scratch collection with an unordered list and a secret.",
		Attrs: []engine.Attr{
			{Name: "name", Kind: engine.String, Required: true, Description: "Its name."},
			{Name: "tags", Kind: engine.StringList, Unordered: true, Description: "Tags, in any order."},
			{Name: "token", Kind: kind, Secret: true, Clearable: true, Description: "The secret."},
			{Name: "token_info", Kind: engine.String, Computed: true, Description: "Derived from whether the secret is held."},
		},
	}
}

func TestARemovedSecretIsClearedWhateverTheSecretsKind(t *testing.T) {
	for name, kind := range map[string]engine.Kind{"string": engine.String, "json": engine.JSON} {
		t.Run(name, func(t *testing.T) {
			f, factories := scratch(t, scratchSecretSpec(kind))
			f.OnWrite("scratch_secret", func(row map[string]any) {
				row["token_info"] = nil
				if set, _ := row["token_set"].(bool); set {
					row["token_info"] = "held"
				}
			})
			value := "tok-1"
			if kind == engine.JSON {
				value = `{"a":1}`
			}
			cfg := func(secret string, version int) string {
				if version == 0 {
					return providerBlock(f.URL) + "resource \"sreagent_scratch_secret\" \"s\" {\n  name = \"s\"\n  tags = [\"b\", \"a\"]\n}\n"
				}
				return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_scratch_secret\" \"s\" {\n  name = \"s\"\n  tags = [\"b\", \"a\"]\n  token_wo = %q\n  token_wo_version = %d\n}\n", secret, version)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{
					{Config: cfg(value, 1), Check: resource.TestCheckResourceAttr("sreagent_scratch_secret.s", "token_info", "held")},
					{
						Config: cfg("", 0),
						ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("sreagent_scratch_secret.s", plancheck.ResourceActionUpdate),
						}},
						Check: resource.TestCheckNoResourceAttr("sreagent_scratch_secret.s", "token_info"),
					},
				},
			})
		})
	}
}
