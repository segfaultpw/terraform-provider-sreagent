package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// SLI mirrors x-sreagent-resources.slis. status is an action argument of the
// update tool, so approving an SLI stays a human act in the app.
var SLI = engine.Spec{
	Key: "slis", TypeName: "sli", ListName: "slis",
	Description: "A service level indicator. Approving one stays a human decision in the app.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Shown on the SLO pages."},
		{Name: "sli_type", Kind: engine.String, Required: true, CreateOnly: true, Description: "latency, availability and so on. Changing it replaces the SLI."},
		{Name: "service", Kind: engine.String, Description: "The service measured."},
		{Name: "query", Kind: engine.String, Description: "The query against the data source."},
		{Name: "data_source_id", Kind: engine.String, Description: "The data source the query runs on."},
		{Name: "description", Kind: engine.String, Description: "What the indicator measures."},
		{Name: "status", Kind: engine.String, Computed: true, Description: "draft, approved or archived; changed in the app."},
	},
}
