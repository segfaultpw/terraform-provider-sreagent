package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// DataSource mirrors x-sreagent-resources.data_sources. enabled is not a
// create argument, so it reaches the platform through a PUT after the POST.
var DataSource = engine.Spec{
	Key: "data_sources", TypeName: "data_source", ListName: "data_sources",
	Description: "A metrics, logs or cloud data source investigations query.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Unique within the organization."},
		{Name: "type", Kind: engine.String, Required: true, CreateOnly: true, Description: "prometheus, loki, cloudwatch and so on. Changing it replaces the source."},
		{Name: "url", Kind: engine.String, Description: "The endpoint; derived for AWS types."},
		{Name: "enabled", Kind: engine.Bool, UpdateOnly: true, Description: "Whether investigations query it."},
		{Name: "regions", Kind: engine.StringList, Description: "AWS regions this source covers."},
		{Name: "auth_type", Kind: engine.String, OneOf: []string{"none", "basic", "bearer", "api_key", "datadog_keys", "aws_access_keys", "aws_assume_role", "aws_instance"}, Description: "How the platform authenticates. Changing it to a type that takes credentials needs auth_credentials_wo with a new auth_credentials_wo_version in the same apply, because the stored credentials belong to the previous type."},
		{Name: "auth_credentials", Kind: engine.JSON, Secret: true, Description: "The credentials for auth_type, as jsonencode({...}). For aws_assume_role send role_arn; the platform stamps the organization's ExternalId."},
		{Name: "role_arn", Kind: engine.String, Computed: true, Description: "The role an aws_assume_role source assumes."},
		{Name: "aws_account_id", Kind: engine.String, Computed: true, Description: "The AWS account the source reads."},
	},
}
