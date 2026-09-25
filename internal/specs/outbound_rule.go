package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// OutboundRule mirrors x-sreagent-resources.outbound_rules.
var OutboundRule = engine.Spec{
	Key: "outbound_rules", TypeName: "outbound_rule", ListName: "outbound_rules",
	Description: "Which alerts page which outbound target, and when.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Unique within the organization."},
		{Name: "outbound_config_id", Kind: engine.String, Required: true, Description: "The target this rule pages."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether the rule is in force."},
		{Name: "match_source", Kind: engine.String, Clearable: true, Description: "Only alerts from this source."},
		{Name: "match_severity", Kind: engine.String, Clearable: true, Description: "Only alerts at this severity."},
		{Name: "match_labels", Kind: engine.JSON, Clearable: true, NullMeans: "{}", Description: "Only alerts carrying these labels, as jsonencode({...}). Unset matches every alert."},
		{Name: "cooldown_minutes", Kind: engine.Int, Clearable: true, NullMeans: "30", Description: "Minimum minutes between pages from this rule; unset means 30."},
		{Name: "escalate_after_minutes", Kind: engine.Int, Clearable: true, Description: "Page only when the alert is still unacknowledged after this many minutes."},
		{Name: "step_order", Kind: engine.Int, Clearable: true, NullMeans: "0", Description: "This rule's step in an escalation chain; unset means 0, the first step."},
		{Name: "outbound_config_name", Kind: engine.String, Computed: true, Description: "The target's name."},
		{Name: "last_triggered_at", Kind: engine.String, Computed: true, Description: "When the rule last paged."},
	},
}
