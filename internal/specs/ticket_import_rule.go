package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// TicketImportRule mirrors x-sreagent-resources.ticket_import_rules: one rule
// per provider, addressed by the provider's name.
var TicketImportRule = engine.Spec{
	Key: "ticket_import_rules", TypeName: "ticket_import_rule", ListName: "ticket_import_rules",
	Description: "What is imported from a ticket provider onto the board; one rule per provider. Destroying it stops imports; imported items stay.",
	Shape:       engine.NaturalKey, IDAttr: "provider", Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "provider", TFName: "provider_type", Kind: engine.String, Required: true, CreateOnly: true, OneOf: []string{"jira", "zoho_sprints", "github"}, Description: "Which tracker."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether imports run."},
		{Name: "config", Kind: engine.JSON, Description: "The rule, as jsonencode({...})."},
		{Name: "last_run_at", Kind: engine.String, Computed: true, Description: "When it last ran."},
		{Name: "last_run_counts", Kind: engine.JSON, Computed: true, Description: "What the last run imported and skipped."},
	},
}
