package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// FixRunnerHealth is data.sreagent_fix_runner_health: whether the organization's
// self-hosted fix runner works, as the state line of the runner card says it.
// The platform publishes no Terraform type for it (Resume, Discard and the
// holding switch are on the runner card and in MCP tools), so it is read only.
var FixRunnerHealth = readSingleton("fix_runner_health",
	"Whether this organization's self-hosted fix runner is working, as the state line of the runner card on the Integrations page says it: working, degraded (its first model is refused and the second does the work), refusing (its AI provider refused three requests in a row, so new requests are routed around it or held), checking (one request is checking whether it works again) or none (no runner is registered). Read only: Resume, Discard and the holding switch are on the runner card and in MCP tools, not in Terraform. Any key the configuration API accepts reads it (an `api:admin` key, or a plan-only `api:config_read` key), and the GitHub integration and the self-hosted runner must be in the plan. `last_refusal` holds the model name, the HTTP status and the AI provider's own words (masked by the platform: no key, account id or ARN); like every value this data source reads, it is stored in the Terraform state, so keep the state where you keep the rest of the organization's configuration. An output built on `state`, `waiting` or `since` changes whenever the runner's health does.",
	"v0.503.0 or later",
	"the organization's plan does not include the GitHub integration or the self-hosted fix runner (the platform answers 403)",
	ro("state", engine.String, "working, degraded, refusing, checking or none."),
	ro("since", engine.String, "When the runner entered its current state, or null."),
	ro("consecutive_refusals", engine.Int, "How many requests in a row the runner's AI provider refused."),
	ro("last_refusal", engine.JSON, "The provider's last refusal, or null: class, class_phrase, model, status, provider_message (the provider's own words, scrubbed, one plain text field) and at."),
	ro("waiting", engine.Int, "How many requests are held for the runner."),
	ro("degraded_since", engine.String, "When the runner started using its second model, or null."),
	ro("hold_while_provider_down", engine.Bool, "Whether requests wait for the runner while its AI provider refuses, instead of being routed around it. True when no runner is registered."),
	ro("next_check_at", engine.String, "The next timed check of a rate limit or quota, or null."),
)
