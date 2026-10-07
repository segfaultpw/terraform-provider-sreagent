package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

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
		{Name: "base_url", Kind: engine.String, Secret: true, Clearable: true, Description: "The endpoint for a webhook or Grafana target; its path or query can carry a token. Never answered: a read answers base_url_set and base_url_host. Leaving base_url_wo out keeps the stored URL; removing base_url_wo and base_url_wo_version after it was set removes the URL."},
		{Name: "api_key", Kind: engine.String, Secret: true, Description: "The bearer token for a webhook or Grafana target."},
		{Name: "routing_key", Kind: engine.String, Secret: true, Description: "The PagerDuty integration routing key."},
		{Name: "base_url_host", Kind: engine.String, Computed: true, Description: "Scheme, host and a non-default port of the URL, never its path or query. Null when no URL is saved."},
		{Name: "rule_count", Kind: engine.Int, Computed: true, Description: "Escalation rules pointing at this target."},
	},
}
