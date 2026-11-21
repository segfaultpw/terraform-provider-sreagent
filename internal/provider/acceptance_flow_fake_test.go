package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// TestAccBoardAndSlackReactionSettings needs a live platform and is not run by `go test`. This test
// runs its steps, built once by reactionSettingsFlow, against the fake facade, so a mistake in that
// flow (an escape, an import that cannot verify, a plan that is not empty) shows here before the
// Acceptance workflow meets it.
func TestTheAcceptanceFlowOfTheReactionSettingsRunsAgainstTheFake(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps:                    reactionSettingsFlow(func(body string) string { return orgSettingsConfig(f.URL, body) }, orgSettings, heldOnFake(f)),
	})
}
