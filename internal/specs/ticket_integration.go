package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// TicketIntegration mirrors x-sreagent-resources.ticket_integrations: one row
// per provider, addressed by the provider's name.
var TicketIntegration = engine.Spec{
	Key: "ticket_integrations", TypeName: "ticket_integration", ListName: "ticket_integrations",
	Description: "The connection to Jira or Zoho Sprints; one per provider. The status map is managed in the app.",
	Shape:       engine.NaturalKey, IDAttr: "provider", Lifecycle: true, Upsert: true,
	Attrs: []engine.Attr{
		{Name: "provider", TFName: "provider_type", Kind: engine.String, Required: true, CreateOnly: true, OneOf: []string{"zoho_sprints", "jira"}, Description: "Which ticket system."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether the connection is used."},
		{Name: "base_url", Kind: engine.String, Description: "The tracker's URL."},
		{Name: "auth_url", Kind: engine.String, Description: "The OAuth endpoint (Zoho)."},
		{Name: "client_id", Kind: engine.String, Description: "The OAuth client id (Zoho)."},
		{Name: "account_email", Kind: engine.String, Description: "The account email (Jira)."},
		{Name: "project_key", Kind: engine.String, Description: "Where new issues are filed."},
		{Name: "issue_type", Kind: engine.String, Description: "The issue type new issues use."},
		{Name: "create_team_id", Kind: engine.String, Description: "Zoho team for new items."},
		{Name: "create_project_id", Kind: engine.String, Description: "Zoho project for new items."},
		{Name: "create_sprint_id", Kind: engine.String, Description: "Zoho sprint for new items."},
		{Name: "client_secret", Kind: engine.String, Secret: true, Description: "The OAuth client secret (Zoho)."},
		{Name: "refresh_token", Kind: engine.String, Secret: true, Description: "The OAuth refresh token (Zoho)."},
		{Name: "api_token", Kind: engine.String, Secret: true, Description: "The API token (Jira)."},
		{Name: "status_map", Kind: engine.JSON, Computed: true, Description: "Board columns to tracker statuses."},
		{Name: "create_ready", Kind: engine.Bool, Computed: true, Description: "Whether new items can be filed."},
	},
}
