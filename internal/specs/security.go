package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// The security and code detection settings are read here and written in the
// app, over MCP or the configuration API; the provider reads them as data
// sources only. Every attribute is Computed, so a spec with a writable field in
// the contract still declares it as an answer.

// readSingleton is a data source for one row per organization that every
// organization has, so a platform that answers 404 predates the resource.
func readSingleton(key, desc, minPlatform string, attrs ...engine.Attr) engine.Spec {
	return engine.Spec{Key: key, TypeName: key, ListName: key, Description: desc, Shape: engine.Singleton, AlwaysPresent: true, MinPlatform: minPlatform, Attrs: attrs}
}

// CodeDetectionSettings is data.sreagent_code_detection_settings.
var CodeDetectionSettings = readSingleton("code_detection_settings",
	"The weekly code detection schedule and scope. Read only: weekly runs spend GitHub requests, so they are switched on in the app.",
	"v0.475.0 or later",
	ro("weekly_enabled", engine.Bool, "Whether the weekly run is on."),
	ro("scan_all", engine.Bool, "Whether a repository with no override is in scope."),
	ro("run_day", engine.Int, "The day of the run, 0 (Sunday) to 6 (Saturday), UTC."),
	ro("run_hour", engine.Int, "The hour of the run, 0 to 23, UTC."),
	ro("default_classes", engine.StringList, "The static kinds the run looks for; empty means every kind."),
	ro("open_ended_enabled", engine.Bool, "Whether the open-ended AI pass is on."),
	ro("available_classes", engine.JSON, "Every static kind that can be named in default_classes."),
	ro("jitter_minutes", engine.Int, "The organization's fixed offset from the run hour, in minutes."),
	ro("manual_runs_today", engine.Int, "Runs a person started today."),
	ro("updated_at", engine.String, "When the settings last changed."),
)

// SecuritySLAPolicy is data.sreagent_security_sla_policy.
var SecuritySLAPolicy = readSingleton("security_sla_policy",
	"The days a security finding may stay active before it breaches, per severity. A severity with no days has no SLA.",
	"v0.476.3 or later",
	ro("critical_days", engine.Int, "Days for a critical finding, 1 to 365, or null for no SLA."),
	ro("high_days", engine.Int, "Days for a high finding, or null."),
	ro("medium_days", engine.Int, "Days for a medium finding, or null."),
	ro("low_days", engine.Int, "Days for a low finding, or null."),
	ro("info_days", engine.Int, "Days for an info finding, or null."),
	ro("defaults", engine.Bool, "True while the organization has not saved a policy of its own and the platform's defaults apply."),
	ro("updated_at", engine.String, "When the policy last changed."),
)

// SecurityDigestSettings is data.sreagent_security_digest_settings.
var SecurityDigestSettings = readSingleton("security_digest_settings",
	"The security digest: how often it is sent and where.",
	"v0.476.4 or later",
	ro("enabled", engine.Bool, "Whether the digest is sent."),
	ro("cadence", engine.String, "daily or weekly."),
	ro("weekday", engine.Int, "For a weekly digest, 1 (Monday) to 7 (Sunday)."),
	ro("hour", engine.Int, "The hour, 0 to 23, in the organization's time zone."),
	ro("slack_channel", engine.String, "The Slack channel, such as #security, or null for none."),
	ro("email", engine.Bool, "Whether the digest is emailed."),
	ro("timezone", engine.String, "The time zone the hour is read in."),
	ro("last_sent_at", engine.String, "When the last digest was sent."),
	ro("last_sent_period", engine.String, "The period the last digest covered."),
)

// SecurityCodeClasses is data.sreagent_security_code_classes.
var SecurityCodeClasses = readSingleton("security_code_classes",
	"The accepted other:<kind> code aliases counted as security findings, beyond the built-in classes.",
	"v0.476.6 or later",
	ro("classes", engine.StringList, "The marked classes; the built-in classes always count and are not listed."),
	ro("available_classes", engine.JSON, "The classes that can be marked."),
	ro("built_in_classes", engine.JSON, "The classes that always count as security findings."),
	ro("limit", engine.Int, "The most classes that can be marked."),
)

// SecurityNotificationRules is data.sreagent_security_notification_rules.
var SecurityNotificationRules = engine.Spec{
	Key: "security_notification_rules", TypeName: "security_notification_rule", ListName: "security_notification_rules",
	Description: "The rules that decide which security findings notify whom. Read only here.",
	Shape:       engine.Generated,
	Attrs: []engine.Attr{
		ro("name", engine.String, "The rule's name, unique in the organization."),
		ro("enabled", engine.Bool, "Whether the rule is on."),
		ro("areas", engine.StringList, "The areas the rule covers; empty means every area."),
		ro("min_severity", engine.String, "The lowest severity that matches."),
		ro("events", engine.StringList, "The events that notify: new_finding, sla_breached, acceptance_expiring, acceptance_expired, fix_verified."),
		ro("slack", engine.Bool, "Whether the rule posts to Slack."),
		ro("slack_channel", engine.String, "The Slack channel, or null for the default channels."),
		ro("email", engine.Bool, "Whether the rule emails."),
		ro("page", engine.Bool, "Whether the rule pages on call (critical findings only)."),
		ro("updated_at", engine.String, "When the rule last changed."),
	},
}

// SecretAllowRules is data.sreagent_secret_allow_rules.
var SecretAllowRules = engine.Spec{
	Key: "secret_allow_rules", TypeName: "secret_allow_rule", ListName: "secret_allow_rules",
	Description: "The rules that keep the weekly repository scan from filing a credential match, by path, repository and detector. Read only here.",
	Shape:       engine.Generated,
	Attrs: []engine.Attr{
		ro("path_glob", engine.String, "The paths the rule covers (** matches any run of characters including /, * any run without /, ? one character)."),
		ro("reason", engine.String, "Why the match is not a credential."),
		ro("repository", engine.String, "owner/name, or null for every repository."),
		ro("detector_type", engine.String, "A detector type, or null for every kind."),
		ro("expires_at", engine.String, "When the rule ends, or null for never."),
		ro("created_at", engine.String, "When the rule was added."),
	},
}
