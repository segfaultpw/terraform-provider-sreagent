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
		{Name: "match_labels", Kind: engine.JSON, Clearable: true, Description: "Only alerts carrying these labels, as jsonencode({...})."},
		{Name: "cooldown_minutes", Kind: engine.Int, Clearable: true, Description: "Minimum minutes between pages from this rule."},
		{Name: "escalate_after_minutes", Kind: engine.Int, Clearable: true, Description: "Page only when the alert is still unacknowledged after this many minutes."},
		{Name: "step_order", Kind: engine.Int, Clearable: true, Description: "This rule's step in an escalation chain."},
		{Name: "outbound_config_name", Kind: engine.String, Computed: true, Description: "The target's name."},
		{Name: "last_triggered_at", Kind: engine.String, Computed: true, Description: "When the rule last paged."},
	},
}
