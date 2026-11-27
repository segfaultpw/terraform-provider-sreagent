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
		{Name: "good_events_query", Kind: engine.String, Description: "A query counting good events, in the data source's own language. Both event queries (this and total_events_query) make an event-ratio SLI. Needs platform v0.529.3. Left out, the value the platform holds stays; this provider cannot remove it."},
		{Name: "total_events_query", Kind: engine.String, Description: "A query counting all events, to pair with good_events_query. Needs platform v0.529.3. Left out, the value the platform holds stays; this provider cannot remove it."},
		{Name: "threshold_good", Kind: engine.Float, Description: "The line between a good sample and a bad one when the SLI has no event queries, in the SLI's unit: at or under it is good for latency, error_rate and saturation, at or over it is good for availability and throughput. Needs platform v0.529.3. Left out, the value the platform holds stays; this provider cannot remove it."},
		{Name: "unit", Kind: engine.String, Description: "The label for the SLI's numbers, such as ms. Needs platform v0.529.3. Left out, the value the platform holds stays; this provider cannot remove it."},
		{Name: "status", Kind: engine.String, Computed: true, Description: "draft, approved or archived; changed in the app."},
	},
}
