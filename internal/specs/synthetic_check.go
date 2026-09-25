package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// SyntheticCheck mirrors x-sreagent-resources.synthetic_checks. The platform
// never answers config.headers or config.body (an HTTP check can carry an
// Authorization header there), and a write replaces config as a whole, so
// Terraform manages the rest of config and leaves those two to the app.
var SyntheticCheck = engine.Spec{
	Key: "synthetic_checks", TypeName: "synthetic_check", ListName: "synthetic_checks",
	Description: "An HTTP, TCP or DNS probe run from the platform's regions. Needs the synthetic checks plan feature.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Shown on the Synthetic Checks page."},
		{Name: "check_type", Kind: engine.String, Required: true, Description: "http, tcp or dns."},
		{Name: "target", Kind: engine.String, Required: true, Description: "The URL, host:port or name probed."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether the check runs."},
		{Name: "interval_seconds", Kind: engine.Int, Description: "Seconds between probes."},
		{Name: "timeout_ms", Kind: engine.Int, Description: "Probe timeout."},
		{Name: "locations", Kind: engine.StringList, Description: "Vantage points; defaults to central."},
		{Name: "service", Kind: engine.String, Description: "The service this check belongs to."},
		{Name: "failure_threshold", Kind: engine.Int, Description: "Consecutive failures before alerting."},
		{Name: "down_quorum", Kind: engine.Int, Description: "Locations that must fail for the check to be down."},
		{Name: "alert_on_failure", Kind: engine.Bool, Description: "Whether a failure raises an alert."},
		{Name: "alert_severity", Kind: engine.String, Description: "critical, high, medium, low or info."},
		{Name: "assertions", Kind: engine.JSON, Description: "Expected status, body match, latency, DNS values, as jsonencode({...})."},
		{
			Name: "config", Kind: engine.JSON,
			HiddenKeys:  []string{"headers", "body"},
			HiddenSetBy: []string{"config_header_names", "config_body_set"},
			Description: "Method, TLS verification, redirects, port, record type, as jsonencode({...}). headers and body are set in the app: the platform never answers them, so they are refused here, and while the check holds either, a change to config is refused because the write would replace them.",
		},
		{Name: "config_header_names", Kind: engine.StringList, Computed: true, Description: "The names of the request headers set in the app; their values are never answered."},
		{Name: "config_body_set", Kind: engine.Bool, Computed: true, Description: "Whether a request body is set in the app; its content is never answered."},
		{Name: "last_status", Kind: engine.String, Computed: true, Description: "The latest probe's result."},
		{Name: "last_run_at", Kind: engine.String, Computed: true, Description: "When the latest probe ran."},
		{Name: "last_error", Kind: engine.String, Computed: true, Description: "The latest probe's error."},
		{Name: "consecutive_failures", Kind: engine.Int, Computed: true, Description: "Failures in a row."},
	},
}
