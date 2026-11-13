package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

const infrastructureRefusal = "The infrastructure suite is not included in your plan."

// wrapped matches parts in order across the line breaks Terraform puts into a
// long diagnostic: any run of white space in a part matches any other.
func wrapped(parts ...string) *regexp.Regexp {
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		quoted = append(quoted, strings.ReplaceAll(regexp.QuoteMeta(p), " ", `\s+`))
	}
	return regexp.MustCompile(`(?s)` + strings.Join(quoted, `.*`))
}

// scratchGated is a managed singleton that needs a plan feature, as the five
// security resources do.
var scratchGated = engine.Spec{
	Key: "scratch_gated", TypeName: "scratch_gated", ListName: "scratch_gated", Shape: engine.Singleton, Lifecycle: true,
	Description: "A scratch singleton that needs the infrastructure suite.",
	PlanFeature: "the infrastructure suite",
	Attrs:       []engine.Attr{{Name: "days", Kind: engine.Int, Description: "Days."}},
}

func TestAPlanRefusalNamesTheFeatureAndTheExits(t *testing.T) {
	f, factories := scratch(t, scratchGated)
	cfg := providerBlock(f.URL) + "resource \"sreagent_scratch_gated\" \"g\" {\n  days = 7\n}\n"
	var posts int
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				// The plan lost the feature: every refresh answers 403.
				PreConfig: func() { f.RefusePlan("scratch_gated", infrastructureRefusal); posts = f.Calls("PUT", "scratch_gated") },
				Config:    cfg,
				PlanOnly:  true,
				ExpectError: wrapped("Could not read sreagent_scratch_gated", "The platform answered 403", infrastructureRefusal,
					"needs the infrastructure suite in the organization's plan", "Nothing was changed.",
					"terraform state rm sreagent_scratch_gated.<name>", "removed block whose lifecycle sets destroy = false"),
			},
			{
				// An apply refused the same way names the feature too, and writes nothing.
				Config:      providerBlock(f.URL) + "resource \"sreagent_scratch_gated\" \"g\" {\n  days = 9\n}\n",
				ExpectError: wrapped("Could not read sreagent_scratch_gated", "infrastructure suite"),
			},
			{
				PreConfig: func() { f.RefusePlan("scratch_gated", ""); f.RefusePlan("scratch_gated", "") },
				Config:    cfg,
				Check: func(*terraform.State) error {
					if got := f.Calls("PUT", "scratch_gated"); got != posts {
						return fmt.Errorf("a refused plan must write nothing, got %d writes more", got-posts)
					}
					return nil
				},
			},
		},
	})
}

func TestAPlanRefusalOfACreateNamesTheFeature(t *testing.T) {
	f, factories := scratch(t, scratchGated)
	f.RefusePlan("scratch_gated", infrastructureRefusal)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(f.URL) + "resource \"sreagent_scratch_gated\" \"g\" {\n  days = 7\n}\n",
			ExpectError: wrapped("Could not create sreagent_scratch_gated", infrastructureRefusal, "terraform state rm"),
		}},
	})
}

func TestAReadOnlyKey403IsNotAPlanRefusal(t *testing.T) {
	f, factories := scratch(t, scratchGated)
	cfg := func(days int) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_scratch_gated\" \"g\" {\n  days = %d\n}\n", days)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg(7)},
			{
				PreConfig: f.ReadOnly,
				Config:    cfg(8),
				// The read-only key keeps its own message, and nothing says the plan is the problem.
				ExpectError: wrapped("api:config_read, which can plan but not apply"),
			},
		},
	})
}

// While the platform refuses every read, a removed block with destroy = false
// still forgets the resource: Terraform plans it without refreshing, so the exit
// the refusal names works. Pinned on the Terraform versions CI runs; if a version
// ever refreshes first, the exit must come out of the refusal's text.
func TestARemovedBlockForgetsARefusedResource(t *testing.T) {
	f, factories := scratch(t, scratchGated)
	cfg := providerBlock(f.URL) + "resource \"sreagent_scratch_gated\" \"g\" {\n  days = 7\n}\n"
	removed := providerBlock(f.URL) + `
removed {
  from = sreagent_scratch_gated.g

  lifecycle {
    destroy = false
  }
}
`
	var reads, writes int
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig: func() {
					f.RefusePlan("scratch_gated", infrastructureRefusal)
					reads, writes = f.Calls("GET", "scratch_gated"), f.Calls("PUT", "scratch_gated")+f.Calls("DELETE", "scratch_gated")
				},
				Config: removed,
				Check: func(s *terraform.State) error {
					if n := len(s.RootModule().Resources); n != 0 {
						return fmt.Errorf("the resource must be forgotten, state still holds %d", n)
					}
					if got := f.Calls("GET", "scratch_gated"); got != reads {
						return fmt.Errorf("a forgotten resource is not refreshed, got %d reads", got-reads)
					}
					if got := f.Calls("PUT", "scratch_gated") + f.Calls("DELETE", "scratch_gated"); got != writes {
						return fmt.Errorf("forgetting a resource writes nothing, got %d writes", got-writes)
					}
					return nil
				},
			},
		},
	})
}
