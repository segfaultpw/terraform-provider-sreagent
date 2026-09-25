package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// DeployPolicy mirrors x-sreagent-resources.deploy_policies.
var DeployPolicy = engine.Spec{
	Key:         "deploy_policies",
	TypeName:    "deploy_policy",
	ListName:    "deploy_policies",
	Description: "A per-service deploy gate policy.",
	Shape:       engine.Generated,
	Lifecycle:   true,
	Attrs: []engine.Attr{
		{Name: "service", Kind: engine.String, Required: true, Description: "The service this policy gates."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether the policy is in force."},
		{Name: "error_budget_threshold", Kind: engine.Float, Description: "Hold deploys while less than this percent of an SLO's error budget is left, 0 to 100; 0 turns the budget block off."},
		{Name: "burn_rate_block_enabled", Kind: engine.Bool, Description: "Hold deploys while an SLO burns faster than its threshold."},
		{Name: "incident_block_enabled", Kind: engine.Bool, Description: "Hold deploys while an incident is open for the service."},
		{Name: "frozen", Kind: engine.Bool, Computed: true, Description: "Whether a freeze is in force now."},
		{Name: "freeze_until", Kind: engine.String, Computed: true, Description: "When the current freeze ends."},
		{Name: "freeze_reason", Kind: engine.String, Computed: true, Description: "Why the service is frozen."},
		{Name: "override_active", Kind: engine.Bool, Computed: true, Description: "Whether an audited override is in force."},
		{Name: "override_allow", Kind: engine.Bool, Computed: true, Description: "The stored override flag; override_active says whether it is in force now."},
		{Name: "override_by", Kind: engine.String, Computed: true, Description: "Who took the override."},
		{Name: "override_until", Kind: engine.String, Computed: true, Description: "When the override ends."},
	},
}
