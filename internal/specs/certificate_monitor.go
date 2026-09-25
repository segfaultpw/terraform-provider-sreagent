package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// CertificateMonitor mirrors x-sreagent-resources.certificate_monitors. There
// is no update tool, so every argument replaces the monitor when it changes.
var CertificateMonitor = engine.Spec{
	Key: "certificate_monitors", TypeName: "certificate_monitor", ListName: "certificate_monitors",
	Description: "Watches a TLS certificate's expiry. Needs the security plan feature. Rows cannot be edited, so any change replaces the monitor.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "hostname", Kind: engine.String, Required: true, CreateOnly: true, Description: "The host to check."},
		{Name: "port", Kind: engine.Int, CreateOnly: true, Description: "1 to 65535; default 443."},
		{Name: "check_interval_hours", Kind: engine.Int, CreateOnly: true, Description: "Hours between checks; default 24."},
		{Name: "warn_days_before", Kind: engine.Int, CreateOnly: true, Description: "Warn this many days before expiry; default 30."},
		{Name: "critical_days_before", Kind: engine.Int, CreateOnly: true, Description: "Critical this many days before expiry."},
		{Name: "enabled", Kind: engine.Bool, Computed: true, Description: "Whether checks run."},
		{Name: "discovered", Kind: engine.Bool, Computed: true, Description: "Whether a sweep created it."},
		{Name: "last_status", Kind: engine.String, Computed: true, Description: "The latest check's verdict."},
		{Name: "last_checked_at", Kind: engine.String, Computed: true, Description: "When it was last checked."},
		{Name: "last_error", Kind: engine.String, Computed: true, Description: "The latest check's error."},
		{Name: "days_until_expiry", Kind: engine.Int, Computed: true, Description: "Days left on the certificate."},
		{Name: "expires_at", Kind: engine.String, Computed: true, Description: "The certificate's notAfter."},
	},
}
