package contract

import (
	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// Stand-ins for the security resources, declared the way their specs will be:
// each one agrees with the vendored document, so a mutation of the document is
// the only thing that can make specProblems object. A drift case uses the real
// spec as soon as that spec is a managed one, and falls back to its stand-in
// until then (see spec in TestTheContractTestCatchesDrift).

func ptr(n int64) *int64 { return &n }

func slaFixture() engine.Spec {
	day := func(name string) engine.Attr {
		return engine.Attr{Name: name, Kind: engine.Int, NullAs: "0", Min: ptr(1), Max: ptr(365)}
	}
	return engine.Spec{
		Key: "security_sla_policy", TypeName: "security_sla_policy", ListName: "security_sla_policy", Shape: engine.Singleton, Lifecycle: true,
		Attrs: []engine.Attr{
			day("critical_days"), day("high_days"), day("medium_days"), day("low_days"), day("info_days"),
			{Name: "defaults", Kind: engine.Bool, Computed: true},
			{Name: "updated_at", Kind: engine.String, Computed: true},
		},
	}
}

const channelPattern = `^#\w+(-\w+)*$`

func digestFixture() engine.Spec {
	return engine.Spec{
		Key: "security_digest_settings", TypeName: "security_digest_settings", ListName: "security_digest_settings", Shape: engine.Singleton, Lifecycle: true,
		Attrs: []engine.Attr{
			{Name: "enabled", Kind: engine.Bool},
			{Name: "cadence", Kind: engine.String, OneOf: []string{"daily", "weekly"}},
			{Name: "weekday", Kind: engine.Int, Min: ptr(1), Max: ptr(7)},
			{Name: "hour", Kind: engine.Int, Min: ptr(0), Max: ptr(23)},
			{Name: "slack_channel", Kind: engine.String, NullAs: `""`, Pattern: channelPattern, MaxLength: 80},
			{Name: "email", Kind: engine.Bool},
			{Name: "timezone", Kind: engine.String, Computed: true},
			{Name: "last_sent_at", Kind: engine.String, Computed: true},
			{Name: "last_sent_period", Kind: engine.String, Computed: true},
		},
	}
}

func codeClassesFixture() engine.Spec {
	return engine.Spec{
		Key: "security_code_classes", TypeName: "security_code_classes", ListName: "security_code_classes", Shape: engine.Singleton, Lifecycle: true,
		Attrs: []engine.Attr{
			{Name: "classes", Kind: engine.StringList, MaxItems: 50},
			{Name: "built_in_classes", Kind: engine.JSON, Computed: true},
			{Name: "available_classes", Kind: engine.JSON, Computed: true},
			{Name: "limit", Kind: engine.Int, Computed: true},
		},
	}
}

func rulesFixture() engine.Spec {
	return engine.Spec{
		Key: "security_notification_rules", TypeName: "security_notification_rule", ListName: "security_notification_rules", Shape: engine.Generated, Lifecycle: true,
		Attrs: []engine.Attr{
			{Name: "name", Kind: engine.String, Required: true, MinLength: 1, MaxLength: 80},
			{Name: "enabled", Kind: engine.Bool, Default: "true"},
			{Name: "areas", Kind: engine.StringList, Default: "[]", OneOf: []string{"secrets", "iam", "kubernetes", "images", "dependencies", "anomalies", "drift", "code", "certificates"}},
			{Name: "min_severity", Kind: engine.String, Default: `"high"`, OneOf: []string{"info", "low", "medium", "high", "critical"}},
			{Name: "events", Kind: engine.StringList, Default: `["new_finding"]`, MinItems: 1, OneOf: []string{"new_finding", "sla_breached", "acceptance_expiring", "acceptance_expired", "fix_verified"}},
			{Name: "slack", Kind: engine.Bool, Default: "true"},
			{Name: "slack_channel", Kind: engine.String, Clearable: true, Pattern: channelPattern, MaxLength: 80},
			{Name: "email", Kind: engine.Bool, Default: "true"},
			{Name: "page", Kind: engine.Bool, Default: "false"},
			{Name: "updated_at", Kind: engine.String, Computed: true},
		},
	}
}

// standIns are the specs a drift case falls back to while the real one is still
// a read-only data source.
var standIns = map[string]func() engine.Spec{
	"security_sla_policy":         slaFixture,
	"security_digest_settings":    digestFixture,
	"security_code_classes":       codeClassesFixture,
	"security_notification_rules": rulesFixture,
}
