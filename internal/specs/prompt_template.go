package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// PromptTemplate mirrors x-sreagent-resources.prompt_templates.
var PromptTemplate = engine.Spec{
	Key: "prompt_templates", TypeName: "prompt_template", ListName: "prompt_templates",
	Description: "A prompt the AI uses. Making one the default clears is_default on the others of its type, which shows as drift on those.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Unique within the organization."},
		{Name: "prompt_type", Kind: engine.String, Required: true, CreateOnly: true, OneOf: []string{"investigation_system", "investigation_user", "slack_assistant", "root_cause", "custom"}, Description: "What the prompt is for; cannot change afterwards."},
		{Name: "content", Kind: engine.String, Required: true, Description: "The prompt text."},
		{Name: "description", Kind: engine.String, Description: "For the people reading the list."},
		{Name: "is_default", Kind: engine.Bool, Description: "Whether this is the default for its type."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether it is used."},
		{Name: "review_status", Kind: engine.String, Computed: true, Description: "The content review's verdict."},
		{Name: "flagged_patterns", Kind: engine.JSON, Computed: true, Description: "Patterns the review flagged."},
	},
}
