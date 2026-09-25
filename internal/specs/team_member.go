package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// TeamMember is one person on one team. One resource per membership rather
// than a member list on sreagent_team, so Terraform never removes somebody
// added in the app and each membership imports on its own.
var TeamMember = engine.Spec{
	Key: "team_members", TypeName: "team_member", ListName: "team_members",
	Description: "One person on one team. The person must already be a member of the organization. Being on a team grants nothing; it decides who is told about the team's work.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "team_id", Kind: engine.String, Required: true, CreateOnly: true, Description: "The team."},
		{Name: "email", Kind: engine.String, Required: true, CreateOnly: true, Normalize: engine.LowerTrim, Description: "The person's sign-in email, matched without case."},
		{Name: "user_id", Kind: engine.String, Computed: true, Description: "The person's user id."},
		{Name: "team_name", Kind: engine.String, Computed: true, Description: "The team's name."},
	},
}
