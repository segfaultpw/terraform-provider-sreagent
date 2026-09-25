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
		{Name: "base_url", Kind: engine.String, Clearable: true, Description: "The endpoint for a webhook or Grafana target."},
		{Name: "slack_channel", Kind: engine.String, Clearable: true, Description: "The channel for a Slack target."},
		{Name: "oncall_schedule_id", Kind: engine.String, Clearable: true, Description: "The schedule for an on-call target."},
		{Name: "default_severity_mapping", Kind: engine.JSON, Clearable: true, Description: "Platform severity to provider severity, as jsonencode({...})."},
		{Name: "api_key", Kind: engine.String, Secret: true, Description: "The bearer token for a webhook or Grafana target."},
		{Name: "routing_key", Kind: engine.String, Secret: true, Description: "The PagerDuty integration routing key."},
		{Name: "rule_count", Kind: engine.Int, Computed: true, Description: "Escalation rules pointing at this target."},
	},
}
