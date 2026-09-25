package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// AlertMute mirrors x-sreagent-resources.alert_mutes. There is no update
// tool, so every argument replaces the mute when it changes.
var AlertMute = engine.Spec{
	Key: "alert_mutes", TypeName: "alert_mute", ListName: "alert_mutes",
	Description: "Silences alerts matching a pattern. Rows cannot be edited, so any change replaces the mute.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "pattern", Kind: engine.String, Required: true, CreateOnly: true, Description: "The substring to silence, 2 to 200 characters."},
		{Name: "reason", Kind: engine.String, CreateOnly: true, Description: "Why it is muted."},
		{Name: "duration_minutes", Kind: engine.Int, NotRead: true, Description: "1 to 10080. Omit for a mute that lasts until it is removed. An expired mute stays in state until you remove it from the configuration."},
		{Name: "ends_at", Kind: engine.String, Computed: true, Description: "When the mute stops silencing."},
		{Name: "created_via", Kind: engine.String, Computed: true, Description: "Which door created it."},
	},
}
