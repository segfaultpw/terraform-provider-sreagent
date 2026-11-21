package provider

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
)

// reactionSettingsFlow is the step list of the acceptance run of the four reaction meanings, the
// column emoji and the two Slack reaction settings. TestAccBoardAndSlackReactionSettings runs it
// against a live platform and TestTheAcceptanceFlowOfTheReactionSettingsRunsAgainstTheFake runs the
// same steps against the fake facade, so the two cannot drift: cfg builds a configuration from the
// body of the resource, addr is its address and held checks what the platform stores for a field,
// given as JSON.
func reactionSettingsFlow(cfg func(body string) string, addr string, held func(field, wantJSON string) resource.TestCheckFunc) []resource.TestStep {
	const first = "  board_column_emoji = {\n    done = \"\\u2705\"\n    in_progress = \"\\U0001F680\"\n  }\n  slack_reaction_sync = \"from_slack\"\n  slack_reaction_counts = false\n  reaction_meanings_off = [\"answer_feedback\", \"tell_author\"]"
	const heartOnly = "  board_column_emoji = {\n    todo = \"\\u2764\"\n  }"
	const defaults = "  board_column_emoji = {}\n  slack_reaction_sync = \"both\"\n  slack_reaction_counts = true\n  reaction_meanings_off = []"
	return []resource.TestStep{
		{
			Config: cfg(first),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(addr, "board_column_emoji.%", "2"),
				resource.TestCheckResourceAttr(addr, "slack_reaction_sync", "from_slack"),
				resource.TestCheckResourceAttr(addr, "slack_reaction_counts", "false"),
				resource.TestCheckResourceAttr(addr, "reaction_meanings_off.#", "2"),
				held("board_column_emoji", `{"done":"\u2705","in_progress":"\ud83d\ude80"}`),
				held("slack_reaction_sync", `"from_slack"`),
				held("slack_reaction_counts", `false`),
				held("reaction_meanings_off", `["answer_feedback","tell_author"]`),
			),
		},
		{Config: cfg(first), ConfigPlanChecks: emptyPlan},
		{ResourceName: addr, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: cfg(first)},
		// A heart spelled without its selector is stored qualified and plans no change.
		{Config: cfg(heartOnly), Check: held("board_column_emoji", `{"todo":"\u2764\ufe0f"}`)},
		{Config: cfg(heartOnly), ConfigPlanChecks: emptyPlan},
		// Left out, everything stays as the platform holds it.
		{Config: cfg("  alert_storm_threshold = 23"), Check: held("board_column_emoji", `{"todo":"\u2764\ufe0f"}`)},
		{
			Config: cfg(defaults),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(addr, "board_column_emoji.%", "0"),
				held("board_column_emoji", `{}`),
				held("slack_reaction_sync", `"both"`),
				held("slack_reaction_counts", `true`),
				held("reaction_meanings_off", `[]`),
			),
		},
	}
}

// heldOnFake checks what the fake platform stores for a field, given as JSON.
func heldOnFake(f *fakefacade.Fake) func(field, wantJSON string) resource.TestCheckFunc {
	return func(field, wantJSON string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			var want any
			if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
				return err
			}
			// The fake keeps Go values; compare through JSON so numbers and lists read alike.
			raw, _ := json.Marshal(orgRow(f)[field])
			var got any
			if err := json.Unmarshal(raw, &got); err != nil {
				return err
			}
			if !reflect.DeepEqual(got, want) {
				return fmt.Errorf("the platform holds %s %s, want %s", field, raw, wantJSON)
			}
			return nil
		}
	}
}
