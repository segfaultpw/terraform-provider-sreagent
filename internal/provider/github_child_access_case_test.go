package provider

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// GitHub spells a repository its own way and the platform folds case, stores GitHub's
// spelling and answers it. A block that writes another case is the same list: the write
// ends without an inconsistent result, the configuration's spelling stays in state and
// the next plan is empty.

func mixedCaseWorld(t *testing.T, mode string, repos ...string) *fakefacade.Fake {
	t.Helper()
	f := fakefacade.New(t, specs.All())
	f.GitHubAccount("acme", "Acme/Web", "Acme/Infra", "Acme/API")
	f.LinkChild("child-1", "acme-labs", "acme", mode, true, repos...)
	return f
}

func TestGitHubChildAccessRepositoriesInAnotherCaseApplyAndPlanNothing(t *testing.T) {
	f := mixedCaseWorld(t, "all")
	cfg := "  mode = \"selected\"\n  repositories = [\"acme/web\", \"ACME/INFRA\"]\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{Config: childAccessConfig(f.URL, "  mode = \"all\"\n")},
			{
				Config: childAccessConfig(f.URL, cfg),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(childAccess, "repositories.#", "2"),
					func(s *terraform.State) error {
						_, repos, _, _ := f.ChildAccessRow("child-1", "acme")
						if !reflect.DeepEqual(repos, []string{"Acme/Web", "Acme/Infra"}) {
							return fmt.Errorf("the platform holds %v, want GitHub's spelling", repos)
						}
						return nil
					},
				),
			},
			{Config: childAccessConfig(f.URL, cfg), ConfigPlanChecks: emptyPlan},
			// The same list written in the platform's own spelling, reordered, is no change either.
			{
				Config:           childAccessConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"Acme/Infra\", \"Acme/Web\"]\n"),
				ConfigPlanChecks: emptyPlan,
			},
		},
	})
}

// State imported from the platform holds its spelling; a block in another case is no change.
func TestGitHubChildAccessImportedSpellingEqualsTheBlocksAnyCase(t *testing.T) {
	f := mixedCaseWorld(t, "selected", "Acme/Web")
	cfg := "  mode = \"selected\"\n  repositories = [\"acme/web\"]\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{
			{ResourceName: childAccess, ImportState: true, ImportStatePersist: true, ImportStateId: "child-1?account=acme", Config: childAccessConfig(f.URL, cfg)},
			{Config: childAccessConfig(f.URL, cfg), ConfigPlanChecks: emptyPlan},
		},
	})
	if n := len(putFingerprints(f)); n != 0 {
		t.Fatalf("an equal list was written %d times", n)
	}
}

func TestGitHubChildAccessRefusesTheSameRepositoryInTwoCases(t *testing.T) {
	f := mixedCaseWorld(t, "all")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   childAccessVersions,
		Steps: []resource.TestStep{{
			Config:      childAccessConfig(f.URL, "  mode = \"selected\"\n  repositories = [\"acme/web\", \"Acme/Web\"]\n"),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`(?s)same\s+value`),
		}},
	})
}
