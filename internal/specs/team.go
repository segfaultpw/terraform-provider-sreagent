package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// Team mirrors x-sreagent-resources.teams. Membership is its own resource,
// sreagent_team_member, so Terraform never owns a whole roster.
var Team = engine.Spec{
	Key: "teams", TypeName: "team", ListName: "teams",
	Description: "A team. Its members are sreagent_team_member resources or are managed in the app.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Unique within the organization."},
		{Name: "members", Kind: engine.JSON, Computed: true, Description: "Current members."},
	},
}
