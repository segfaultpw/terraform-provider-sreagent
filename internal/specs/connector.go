package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// Connector mirrors x-sreagent-resources.connectors. The whole config map is
// stored encrypted, so it is write-only as a whole.
var Connector = engine.Spec{
	Key: "connectors", TypeName: "connector", ListName: "connectors",
	Description: "An infrastructure connector runbooks act through.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Unique within the organization."},
		{Name: "connector_type", Kind: engine.String, Required: true, CreateOnly: true, Description: "aws_ec2, ssh, kubernetes and so on. Changing it replaces the connector."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether runbooks may use it."},
		{Name: "metadata", Kind: engine.JSON, Description: "Free-form labels, as jsonencode({...})."},
		{Name: "config", Kind: engine.JSON, Secret: true, Required: true, Description: "The connector's settings and credentials, as jsonencode({...}). Required to create a connector. Stored encrypted as a whole, so it is write-only as a whole."},
		{Name: "health_status", Kind: engine.String, Computed: true, Description: "The latest health check's verdict."},
	},
}
