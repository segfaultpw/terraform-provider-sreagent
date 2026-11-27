package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// cardEventKinds are the groups of card events a webhook target can choose.
var cardEventKinds = []string{"approvals", "created", "moved", "assignees", "comments", "checklist", "relations", "closed"}

// OutboundConfig mirrors x-sreagent-resources.outbound_configs.
var OutboundConfig = engine.Spec{
	Key: "outbound_configs", TypeName: "outbound_config", ListName: "outbound_configs",
	Description: "An escalation target: PagerDuty, Grafana, a webhook, Slack or an on-call schedule.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Unique within the organization."},
		{Name: "provider_type", Kind: engine.String, Required: true, OneOf: []string{"pagerduty", "grafana", "webhook", "slack", "oncall_schedule"}, Description: "Which system this target reaches."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether the target receives pages."},
		{Name: "priority", Kind: engine.Int, Description: "Order among targets."},
		{Name: "slack_channel", Kind: engine.String, Clearable: true, Description: "The channel for a Slack target."},
		{Name: "oncall_schedule_id", Kind: engine.String, Clearable: true, Description: "The schedule for an on-call target."},
		{Name: "default_severity_mapping", Kind: engine.JSON, Clearable: true, NullMeans: "{}", Description: "Platform severity to provider severity, as jsonencode({...}). Unset uses the built-in mapping."},
		{Name: "card_events", Kind: engine.Bool, Description: "Webhook targets only: also send the organization's card approval events here. Refused for any other provider_type."},
		{Name: "card_event_kinds", Kind: engine.StringList, Unordered: true, OneOf: cardEventKinds, Description: "Webhook targets only: which groups of card events this target receives, as the whole set. approvals (card.approval_requested and card.approval_decided) is the default and the only group a target hears until more are chosen; created, moved, assignees, comments, checklist, relations and closed add card.created, card.moved, card.assignees_changed, card.commented and card.comment_deleted, card.checklist_changed, the link and child events, and card.closed and card.reopened. Needs at least one while card_events is on. The order does not matter. A target receives only what happens after card_events or enabled was switched on, or its first kind other than approvals was chosen; adding a further kind does not move that moment. Refused for any other provider_type. Needs platform v0.522.0."},
		{Name: "card_event_text", Kind: engine.Bool, Description: "Webhook targets only: include comment bodies, checklist item text and notes and a won't-do reason in the card events sent here. Off by default: an event then carries ids, key, title, column and who acted, never text. Turn it on only when the receiver should hold that text. Refused for any other provider_type. Needs platform v0.522.0."},
		{Name: "base_url", Kind: engine.String, Secret: true, Clearable: true, Description: "The endpoint for a webhook or Grafana target; its path or query can carry a token. Never answered: a read answers base_url_set and base_url_host. Leaving base_url_wo out keeps the stored URL; removing base_url_wo and base_url_wo_version after it was set removes the URL."},
		{Name: "api_key", Kind: engine.String, Secret: true, Description: "The bearer token for a webhook or Grafana target."},
		{Name: "routing_key", Kind: engine.String, Secret: true, Description: "The PagerDuty integration routing key."},
		{Name: "base_url_host", Kind: engine.String, Computed: true, Description: "Scheme, host and a non-default port of the URL, never its path or query. Null when no URL is saved."},
		{Name: "rule_count", Kind: engine.Int, Computed: true, Description: "Escalation rules pointing at this target."},
	},
}
