package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// StatusPageComponent mirrors x-sreagent-resources.status_page_components.
// move is an action on the update tool, never an attribute.
var StatusPageComponent = engine.Spec{
	Key: "status_page_components", TypeName: "status_page_component", ListName: "status_page_components",
	Description: "A component on the public status page. Destroying it archives it. Order is managed in the app.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "display_name", Kind: engine.String, Required: true, Description: "Shown on the public page."},
		{Name: "description", Kind: engine.String, Clearable: true, Description: "Shown under the name."},
		{Name: "slo_ids", Kind: engine.StringList, Description: "SLOs whose state colours the component."},
		{Name: "position", Kind: engine.Int, Computed: true, Description: "The component's place on the page."},
	},
}
