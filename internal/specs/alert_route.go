package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// AlertRoute mirrors x-sreagent-resources.alert_routes. The platform stores
// service names trimmed and lowercased; PUT cannot change a route's service.
var AlertRoute = engine.Spec{
	Key:         "alert_routes",
	TypeName:    "alert_route",
	ListName:    "alert_routes",
	Description: "Where one service's alerts open in Slack.",
	Shape:       engine.Generated,
	Lifecycle:   true,
	Attrs: []engine.Attr{
		{Name: "service", Kind: engine.String, Required: true, CreateOnly: true, Normalize: engine.LowerTrim, Description: "The service whose alerts this routes. Changing it replaces the route."},
		{Name: "target", Kind: engine.String, Required: true, Description: "The Slack channel this service's alerts open in."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether the route is in force."},
		{Name: "oncall_schedule_id", Kind: engine.String, Clearable: true, Description: "An on-call schedule whose current person is mentioned in the alert's thread. Removing it clears the mention."},
	},
}
