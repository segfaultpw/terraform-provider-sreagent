package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// SLO mirrors x-sreagent-resources.slos.
var SLO = engine.Spec{
	Key: "slos", TypeName: "slo", ListName: "slos",
	Description: "A service level objective over one SLI.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Shown on the SLO pages."},
		{Name: "sli_id", Kind: engine.String, Required: true, CreateOnly: true, Description: "The indicator. Changing it replaces the SLO."},
		{Name: "target", Kind: engine.Float, Required: true, Description: "The objective, for example 99.9."},
		{Name: "window_days", Kind: engine.Int, Description: "The rolling window; defaults to 30."},
		{Name: "status", Kind: engine.String, Computed: true, Description: "Lifecycle status; changed in the app."},
	},
}
