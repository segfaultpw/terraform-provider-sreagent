package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// AIProvider mirrors x-sreagent-resources.ai_providers. A provider inherited
// from a parent organization reads as not found.
var AIProvider = engine.Spec{
	Key: "ai_providers", TypeName: "ai_provider", ListName: "ai_providers",
	Description: "A model provider the organization brings. A provider inherited from a parent organization is not visible here.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Unique within the organization."},
		{Name: "provider", TFName: "provider_type", Kind: engine.String, Required: true, OneOf: []string{"anthropic", "openai", "openrouter", "ollama", "bedrock", "gemini", "portkey", "langchain"}, Description: "The provider."},
		{Name: "model", Kind: engine.String, Clearable: true, Description: "The default model."},
		{Name: "base_url", Kind: engine.String, Clearable: true, Description: "A custom endpoint."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether it is routed to."},
		{Name: "priority", Kind: engine.Int, Description: "Routing order."},
		{Name: "auto_upgrade_model", Kind: engine.Bool, Description: "Follow the provider's newest model."},
		{Name: "model_overrides", Kind: engine.JSON, Description: "Per call type models, as jsonencode({...})."},
		{Name: "max_tokens", Kind: engine.Int, Clearable: true, NullMeans: "4096", Description: "Output token cap; unset means 4096."},
		{Name: "temperature", Kind: engine.Float, Clearable: true, NullMeans: "0.7", Description: "Sampling temperature; a new provider starts at 0.7."},
		{Name: "timeout_ms", Kind: engine.Int, Clearable: true, NullMeans: "60000", Description: "Request timeout; a new provider starts at 60000."},
		{Name: "rate_limit_rpm", Kind: engine.Int, Clearable: true, Description: "Requests per minute."},
		{Name: "api_key", Kind: engine.String, Secret: true, Description: "The provider's API key."},
	},
}
