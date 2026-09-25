package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// ServiceBinding mirrors x-sreagent-resources.service_bindings, addressed by
// service plus an optional environment rather than by a generated id.
var ServiceBinding = engine.Spec{
	Key: "service_bindings", TypeName: "service_binding", ListName: "service_bindings",
	Description: "Which log groups, traces and metrics belong to a service in the Observability Explorer.",
	Shape:       engine.ServiceBinding, Lifecycle: true, Upsert: true,
	Attrs: []engine.Attr{
		{Name: "service", Kind: engine.String, Required: true, CreateOnly: true, Normalize: engine.LowerTrim, Description: "The service. Stored trimmed and lowercased."},
		{Name: "environment", Kind: engine.String, CreateOnly: true, Normalize: engine.LowerTrim, Description: "The environment; empty for the default."},
		{Name: "log_groups", Kind: engine.StringList, Description: "CloudWatch log groups."},
		{Name: "log_filter", Kind: engine.String, Description: "A filter pattern applied to them."},
		{Name: "trace_service_names", Kind: engine.StringList, Description: "Service names in traces."},
		{Name: "metric_selectors", Kind: engine.JSON, Description: "Metric selectors, as jsonencode([...])."},
		{Name: "account_id", Kind: engine.String, Clearable: true, Description: "The AWS account the service lives in; when set, logs and metrics are searched there only. Traces always search every account."},
		{Name: "region", Kind: engine.String, Clearable: true, Description: "The AWS region the service lives in; pairs with account_id."},
	},
}
