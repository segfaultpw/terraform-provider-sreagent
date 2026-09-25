package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// ImageTarget mirrors x-sreagent-resources.image_targets. There is no update
// tool, so a change replaces the target; one a discovery sweep filed comes
// back on the next sweep after a destroy.
var ImageTarget = engine.Spec{
	Key: "image_targets", TypeName: "image_target", ListName: "image_targets",
	Description: "A container image scanned for CVEs. Needs the security plan feature. Rows cannot be edited, so a change replaces the target. An image a cluster or ECS discovery filed comes back on the next sweep after a destroy.",
	Shape:       engine.Generated, Lifecycle: true, DiscoveredAttr: "discovered",
	Attrs: []engine.Attr{
		{Name: "image_ref", Kind: engine.String, Required: true, CreateOnly: true, Description: "The image, for example nginx:1.27."},
		{Name: "enabled", Kind: engine.Bool, Computed: true, Description: "Whether scans run."},
		{Name: "scan_interval_hours", Kind: engine.Int, Computed: true, Description: "Hours between scans."},
		{Name: "discovered", Kind: engine.Bool, Computed: true, Description: "Whether a sweep filed it."},
		{Name: "last_scan_status", Kind: engine.String, Computed: true, Description: "The latest scan's verdict."},
		{Name: "last_scanned_at", Kind: engine.String, Computed: true, Description: "When the latest scan ran."},
		{Name: "critical_count", Kind: engine.Int, Computed: true, Description: "Critical CVEs."},
		{Name: "high_count", Kind: engine.Int, Computed: true, Description: "High CVEs."},
		{Name: "medium_count", Kind: engine.Int, Computed: true, Description: "Medium CVEs."},
		{Name: "low_count", Kind: engine.Int, Computed: true, Description: "Low CVEs."},
		{Name: "scanned_at", Kind: engine.String, Computed: true, Description: "When it was last scanned."},
	},
}
