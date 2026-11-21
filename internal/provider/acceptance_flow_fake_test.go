package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// TestAccBoardAndSlackReactionSettings needs a live platform and is not run by `go test`. This test
// runs its steps, the same configurations in the same order, against the fake facade, so a mistake
// in that flow (an escape, an import that cannot verify, a plan that is not empty) shows here
// before the Acceptance workflow meets it.
func TestTheAcceptanceFlowOfTheReactionSettingsRunsAgainstTheFake(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(body string) string { return orgSettingsConfig(f.URL, body) }
	const first = "  board_column_emoji = {\n    done = \"\\u2705\"\n    in_progress = \"\\U0001F680\"\n  }\n  slack_reaction_sync = \"from_slack\"\n  slack_reaction_counts = false\n  reaction_meanings_off = [\"answer_feedback\", \"tell_author\"]"
	const heartOnly = "  board_column_emoji = {\n    todo = \"\\u2764\"\n  }"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg(first),
				Check: resource.ComposeAggregateTestCheckFunc(
					holds(f, "board_column_emoji", map[string]any{"done": "\u2705", "in_progress": "\U0001F680"}),
					holds(f, "slack_reaction_sync", "from_slack"),
					holds(f, "slack_reaction_counts", false),
					holds(f, "reaction_meanings_off", []any{"answer_feedback", "tell_author"}),
				),
			},
			{Config: cfg(first), ConfigPlanChecks: emptyPlan},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: cfg(first)},
			// A heart spelled without its selector is stored qualified and plans no change.
			{Config: cfg(heartOnly), Check: holds(f, "board_column_emoji", map[string]any{"todo": "\u2764\ufe0f"})},
			{Config: cfg(heartOnly), ConfigPlanChecks: emptyPlan},
			// Left out, everything stays as the platform holds it.
			{Config: cfg("  alert_storm_threshold = 23"), Check: holds(f, "board_column_emoji", map[string]any{"todo": "\u2764\ufe0f"})},
			{
				Config: cfg("  board_column_emoji = {}\n  slack_reaction_sync = \"both\"\n  slack_reaction_counts = true\n  reaction_meanings_off = []"),
				Check: resource.ComposeAggregateTestCheckFunc(
					holds(f, "board_column_emoji", map[string]any{}),
					holds(f, "slack_reaction_sync", "both"),
					holds(f, "slack_reaction_counts", true),
					holds(f, "reaction_meanings_off", []any{}),
				),
			},
		},
	})
}
