package specs

import (
	"slices"
	"strings"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// The security and code detection settings are read here and written in the
// app, over MCP or the configuration API; the provider reads them as data
// sources only. Every attribute is Computed, so a spec with a writable field in
// the contract still declares it as an answer.

// infrastructureRefusal is the plan gate of the security tools: the facade answers 403 with the
// platform's sentence for an organization whose plan lacks the infrastructure suite.
const infrastructureRefusal = "the organization's plan does not include the infrastructure suite (the platform answers 403)"

// readSingleton is a data source for one row per organization that has the feature, so a
// platform that answers 404 predates the resource. The security singletons are gated by the
// infrastructure suite, which answers 403, never 404.
func readSingleton(key, desc, minPlatform, refusedWhen string, attrs ...engine.Attr) engine.Spec {
	return engine.Spec{Key: key, TypeName: key, ListName: key, Description: desc, Shape: engine.Singleton, AlwaysPresent: true, MinPlatform: minPlatform, RefusedWhen: refusedWhen, Attrs: attrs}
}

// CodeDetectionSettings is data.sreagent_code_detection_settings. Code improvement is not
// every platform's or every organization's: the platform answers 404 when the feature is
// not offered (no verifier configured, or the organization outside the rollout), and 404
// again with its own sentence when the plan lacks code improvement or no verifier is set up.
var CodeDetectionSettings = withAbsentWhen(readSingleton("code_detection_settings",
	"The weekly code detection schedule and scope. Read only: weekly runs spend GitHub requests, so they are switched on in the app. Only organizations that are offered code improvement have these settings; the read is an error for the others.",
	"v0.475.0 or later", "",
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
),
	"the platform does not offer code improvement (no verifier is configured), the organization is outside its rollout, "+
		"its plan does not include code improvement, or code improvement is not set up for it (the platform answers 404 for each)")

// withAbsentWhen names the conditions under which the platform answers 404 beside being older.
func withAbsentWhen(s engine.Spec, absentWhen string) engine.Spec {
	s.AbsentWhen = absentWhen
	return s
}

// The security settings and rules are managed here: the SLA policy, the digest and the code
// classes are singletons (leaving an attribute out leaves it as it is), the notification
// rules and the secret allow rules are collections. All five need the infrastructure suite,
// which the platform answers with a 403 for every verb, a read included.
const infrastructureSuite = "the infrastructure suite"

// securityNote is what a managed security singleton says about its destroy.
const securityNote = " As a resource it adopts the settings the organization has: an attribute left out is left as it is, " +
	"and destroying the resource leaves the settings in place (the platform has no row to delete). " +
	"Needs the infrastructure suite."

func ptr(n int64) *int64 { return &n }

// dataSourceNote is added to what the data source says of a managed security resource.
func dataSourceNote(resourceType string) string {
	return " This data source only reads; sreagent_" + resourceType + " manages it."
}

// channelPattern is the platform's Slack channel name: a hash and words joined by hyphens.
const channelPattern = `^#\w+(-\w+)*$`

// slaDay is one severity's days. 0 stands for no SLA, which the platform stores as null.
func slaDay(name, article, severity, read string) engine.Attr {
	return engine.Attr{Name: name, Kind: engine.Int, NullAs: "0", Min: ptr(1), Max: ptr(365),
		ChangeWarning:   "Changing the SLA recomputes the due date of every active finding of this severity; a due date a person moved is kept.",
		Description:     "Days " + article + " " + severity + " finding may stay active before it breaches, 1 to 365, or 0 for no SLA (the platform stores no SLA as null, which the data source reads as null).",
		ReadDescription: read}
}

// SecuritySLAPolicy is data.sreagent_security_sla_policy and sreagent_security_sla_policy.
var SecuritySLAPolicy = func() engine.Spec {
	sp := readSingleton("security_sla_policy",
		"The days a security finding may stay active before it breaches, per severity. The platform's defaults are 7 days for critical, 30 for high, 90 for medium, 180 for low and no SLA for info; a severity with no days has no SLA."+
			securityNote+" Changing a day recomputes the due date of every active finding of that severity. To go back to the defaults, state them (7, 30, 90, 180 and 0), apply, then remove the block.",
		"v0.476.3 or later", infrastructureRefusal,
		slaDay("critical_days", "a", "critical", "Days for a critical finding, 1 to 365, or null for no SLA."),
		slaDay("high_days", "a", "high", "Days for a high finding, or null."),
		slaDay("medium_days", "a", "medium", "Days for a medium finding, or null."),
		slaDay("low_days", "a", "low", "Days for a low finding, or null."),
		slaDay("info_days", "an", "info", "Days for an info finding, or null."),
		ro("defaults", engine.Bool, "True while the organization has not saved a policy of its own and the platform's defaults apply."),
		ro("updated_at", engine.String, "When the policy last changed."),
	)
	sp.Lifecycle = true
	sp.PlanFeature = infrastructureSuite
	sp.ReadDescription = "The days a security finding may stay active before it breaches, per severity. A severity with no days has no SLA." + dataSourceNote("security_sla_policy")
	return sp
}()

// digestDestination is the platform's rule that a digest which is on has somewhere to go.
func digestDestination(plan, _ engine.Config) []engine.Issue {
	enabled, on := plan.Bool("enabled")
	channel, ch := plan.String("slack_channel")
	email, em := plan.Bool("email")
	if on != engine.Present || !enabled || ch == engine.Unknown || em == engine.Unknown {
		return nil
	}
	if channel == "" && (em != engine.Present || !email) {
		return []engine.Issue{{Attr: "enabled", Summary: "No destination for the digest",
			Detail: "Choose a Slack channel or email for the digest. Set slack_channel or email = true, or enabled = false."}}
	}
	return nil
}

// SecurityDigestSettings is data.sreagent_security_digest_settings and sreagent_security_digest_settings.
var SecurityDigestSettings = func() engine.Spec {
	sp := readSingleton("security_digest_settings",
		"The security digest: how often it is sent and where. It is off by default; switching it on needs a Slack channel or email."+
			securityNote+" Destroying the resource does not switch the digest off: it keeps sending while enabled is true. To go back to the defaults, state them (enabled = false, cadence = \"weekly\", weekday = 1, hour = 9, slack_channel = \"\", email = false), apply, then remove the block.",
		"v0.476.4 or later", infrastructureRefusal,
		engine.Attr{Name: "enabled", Kind: engine.Bool, Description: "Whether the digest is sent. Needs a Slack channel or email.", ReadDescription: "Whether the digest is sent."},
		engine.Attr{Name: "cadence", Kind: engine.String, OneOf: []string{"daily", "weekly"}, Description: "How often it is sent.", ReadDescription: "daily or weekly."},
		engine.Attr{Name: "weekday", Kind: engine.Int, Min: ptr(1), Max: ptr(7), Description: "For a weekly digest, 1 (Monday) to 7 (Sunday)."},
		engine.Attr{Name: "hour", Kind: engine.Int, Min: ptr(0), Max: ptr(23), Description: "The hour, 0 to 23, in the organization's time zone."},
		engine.Attr{Name: "slack_channel", Kind: engine.String, NullAs: `""`, Pattern: channelPattern, MaxLength: 80,
			Description:     "The Slack channel, such as #security, or an empty string for none (the platform stores no channel as null, which the data source reads as null). There is no default channel for a digest.",
			ReadDescription: "The Slack channel, such as #security, or null for none."},
		engine.Attr{Name: "email", Kind: engine.Bool, Description: "Whether the digest is emailed to the organization's notification list.", ReadDescription: "Whether the digest is emailed."},
		ro("timezone", engine.String, "The time zone the hour is read in."),
		ro("last_sent_at", engine.String, "When the last digest was sent."),
		ro("last_sent_period", engine.String, "The period the last digest covered."),
	)
	sp.Lifecycle = true
	sp.PlanFeature = infrastructureSuite
	sp.PlanChecks = []func(plan, prior engine.Config) []engine.Issue{digestDestination}
	sp.ReadDescription = "The security digest: how often it is sent and where." + dataSourceNote("security_digest_settings")
	return sp
}()

// codeClassesCheck refuses what the platform refuses from the accepted classes the last read
// reported: a built-in class, and a class the organization never accepted. A create has no
// prior read, so the platform judges it.
func codeClassesCheck(plan, prior engine.Config) []engine.Issue {
	if prior == nil {
		return nil
	}
	classes, ok := plan.Strings("classes")
	if ok != engine.Present {
		return nil
	}
	builtIn, b := prior.Strings("built_in_classes")
	available, a := prior.Strings("available_classes")
	marked, m := prior.Strings("classes")
	if b != engine.Present || a != engine.Present {
		return nil
	}
	var out []engine.Issue
	for _, c := range classes {
		switch {
		case slices.Contains(builtIn, c):
			out = append(out, engine.Issue{Attr: "classes", Summary: "A built-in class is not listed",
				Detail: c + " always counts as security and is not listed; list the other:<kind> classes you want counted."})
		case m == engine.Present && slices.Contains(marked, c), slices.Contains(available, c):
		default:
			named := "none"
			if len(available) > 0 {
				named = strings.Join(available, ", ")
			}
			out = append(out, engine.Issue{Attr: "classes", Summary: "A class the organization has not accepted",
				Detail: c + " is not a class this organization accepted. Name a class from available_classes: " + named + "."})
		}
	}
	return out
}

// SecurityCodeClasses is data.sreagent_security_code_classes and sreagent_security_code_classes.
var SecurityCodeClasses = func() engine.Spec {
	sp := readSingleton("security_code_classes",
		"The accepted other:<kind> code aliases counted as security findings, beyond the four built-in classes (hardcoded_secret, unsafe_deserialization, ci_unpinned_action and ci_plain_secret), which always count and are never listed."+
			securityNote+" classes is the complete list of marked classes: a class left out is unmarked, and [] unmarks them all. Unmarking a class closes its open security findings; marking it again restores them. Destroying the resource leaves the marks in place.",
		"v0.476.6 or later", infrastructureRefusal,
		engine.Attr{Name: "classes", Kind: engine.StringList, Unordered: true, MaxItems: 50,
			RemovedWarning:  "Unmarking %s closes its open security findings as source_removed, which ends an acceptance on them; marking it again restores them.",
			Description:     "The marked classes, in any order: only a class from available_classes (an other:<kind> alias accepted on the Code improvement page) or one already marked, at most 50.",
			ReadDescription: "The marked classes; the built-in classes always count and are not listed."},
		ro("available_classes", engine.JSON, "The classes that can be marked."),
		ro("built_in_classes", engine.JSON, "The classes that always count as security findings."),
		ro("limit", engine.Int, "The most classes that can be marked."),
	)
	sp.Lifecycle = true
	sp.PlanFeature = infrastructureSuite
	sp.PlanChecks = []func(plan, prior engine.Config) []engine.Issue{codeClassesCheck}
	sp.ReadDescription = "The accepted other:<kind> code aliases counted as security findings, beyond the built-in classes." + dataSourceNote("security_code_classes")
	return sp
}()

var (
	ruleAreas      = []string{"secrets", "iam", "kubernetes", "images", "dependencies", "anomalies", "drift", "code", "certificates"}
	ruleSeverities = []string{"info", "low", "medium", "high", "critical"}
	ruleEvents     = []string{"new_finding", "sla_breached", "acceptance_expiring", "acceptance_expired", "fix_verified"}
)

// ruleIssues is the platform's two rules across a notification rule's attributes: paging only
// on a critical rule, and somewhere to send. A value the configuration leaves out is the
// platform's default, which is what the plan carries too.
func ruleIssues(c engine.Config) []engine.Issue {
	page, pg := c.Bool("page")
	severity, sv := c.String("min_severity")
	slack, sl := c.Bool("slack")
	email, em := c.Bool("email")
	if pg == engine.Absent {
		page, pg = false, engine.Present
	}
	if sv == engine.Absent {
		severity, sv = "high", engine.Present
	}
	if sl == engine.Absent {
		slack, sl = true, engine.Present
	}
	if em == engine.Absent {
		email, em = true, engine.Present
	}
	var out []engine.Issue
	if pg == engine.Present && page && sv == engine.Present && severity != "critical" {
		out = append(out, engine.Issue{Attr: "page", Summary: "Paging needs a critical rule",
			Detail: "Paging is only allowed on a rule whose minimum severity is critical. Set min_severity = \"critical\", or page = false."})
	}
	if pg == engine.Present && sl == engine.Present && em == engine.Present && !page && !slack && !email {
		out = append(out, engine.Issue{Attr: "slack", Summary: "No destination", Detail: "Choose at least one of Slack, email or paging."})
	}
	return out
}

// SecurityNotificationRules is data.sreagent_security_notification_rules and
// sreagent_security_notification_rule.
var SecurityNotificationRules = engine.Spec{
	Key: "security_notification_rules", TypeName: "security_notification_rule", ListName: "security_notification_rules",
	Description: "A rule that decides which security findings notify whom: its areas, the lowest severity, the events, and where it goes (Slack, email, paging). " +
		"An attribute left out takes the platform's default (on, every area, minimum severity high, the new_finding event, Slack and email on, paging off), so removing page = true stops paging. " +
		"An organization has at most 20 rules, each name unique; the platform refuses the 21st at apply. " +
		"The organization's first rule, Critical and high findings, is created once by the platform: import it to manage it (its events are new_finding, sla_breached, acceptance_expired and fix_verified, so a block that leaves events out plans a change to new_finding). " +
		"Destroying a rule deletes it; destroying every rule, the seeded one included, is permanent: the platform never creates it again, and an organization with no rule is sent no security notice. " +
		"Needs the infrastructure suite.",
	Shape:       engine.Generated,
	Lifecycle:   true,
	PlanFeature: infrastructureSuite,
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Trim: true, MinLength: 1, MaxLength: 80, Description: "The rule's name, 1 to 80 characters, unique in the organization.", ReadDescription: "The rule's name, unique in the organization."},
		{Name: "enabled", Kind: engine.Bool, Default: "true", Description: "Whether the rule is on. Defaults to true.", ReadDescription: "Whether the rule is on."},
		{Name: "areas", Kind: engine.StringList, OneOf: ruleAreas, Default: "[]", Description: "The areas the rule covers; empty means every area (the default).", ReadDescription: "The areas the rule covers; empty means every area."},
		{Name: "min_severity", Kind: engine.String, OneOf: ruleSeverities, Default: `"high"`, Description: "The lowest severity that matches. Defaults to high.", ReadDescription: "The lowest severity that matches."},
		{Name: "events", Kind: engine.StringList, OneOf: ruleEvents, MinItems: 1, Default: `["new_finding"]`, Description: "The events that notify, at least one. Defaults to new_finding.", ReadDescription: "The events that notify: new_finding, sla_breached, acceptance_expiring, acceptance_expired, fix_verified."},
		{Name: "slack", Kind: engine.Bool, Default: "true", Description: "Whether the rule posts to Slack. Defaults to true.", ReadDescription: "Whether the rule posts to Slack."},
		{Name: "slack_channel", Kind: engine.String, Clearable: true, Pattern: channelPattern, MaxLength: 80, Description: "The Slack channel, such as #security-alerts. Leave it out for the organization's default channels.", ReadDescription: "The Slack channel, or null for the default channels."},
		{Name: "email", Kind: engine.Bool, Default: "true", Description: "Whether the rule emails. Defaults to true.", ReadDescription: "Whether the rule emails."},
		{Name: "page", Kind: engine.Bool, Default: "false", Description: "Whether the rule pages on call. Only a rule whose min_severity is critical may page, and only new findings page. Defaults to false.", ReadDescription: "Whether the rule pages on call (critical findings only)."},
		ro("updated_at", engine.String, "When the rule last changed."),
	},
	ReadDescription: "The rules that decide which security findings notify whom." + dataSourceNote("security_notification_rule"),
	Checks:          []func(engine.Config) []engine.Issue{ruleIssues},
	PlanChecks:      []func(plan, prior engine.Config) []engine.Issue{func(plan, _ engine.Config) []engine.Issue { return ruleIssues(plan) }},
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
