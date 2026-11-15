package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// AlertFollowUpSettings is data.sreagent_alert_follow_up_settings. The platform
// does not publish a Terraform type for it (an organization admin switches the
// follow-up on in Settings, and it starts off), so it is read only here, like the
// code detection settings were before the platform typed them.
var AlertFollowUpSettings = engine.Spec{
	Key: "alert_follow_up_settings", TypeName: "alert_follow_up_settings", ListName: "alert_follow_up_settings",
	Description: "The alert follow-up setting, as the Settings card shows it: whether SRE Agent checks alerts that stay open, what it does with one its source says has cleared, the ages at which it first checks, reminds, opens a card and escalates, the kinds of alert that are only being suggested for now, the informative kinds, and what the follow-up did in the last 7 days. Read only: an organization admin switches it on and changes it in Settings (or with the MCP tool update_alert_follow_up_settings), and it is off until then. Available on every plan.",
	Shape:       engine.Singleton,
	// Every organization has this setting, off until an admin switches it on, so a
	// 404 is a platform older than the resource.
	AlwaysPresent: true,
	MinPlatform:   "v0.489.0 or later",
	Attrs: []engine.Attr{
		ro("enabled", engine.Bool, "Whether SRE Agent checks alerts that stay open. Off until an organization admin switches it on."),
		ro("still_active_enabled", engine.Bool, "Whether SRE Agent also chases an alert that stays open with nobody on it: after the nudge, card and escalate ages it reminds the person on call, opens a card, and can page the person on call (or the next step of the escalation chain) for a critical or high alert its source still reports as firing. Off until an organization admin switches it on, and it needs enabled to do anything."),
		ro("informative_expire_hours", engine.Int, "The age a newly approved informative kind closes its alerts after, in hours."),
		ro("resolve_modes", engine.JSON, "What the follow-up does with an alert its source says has cleared, by severity: auto resolves it, suggest only offers Resolve, off does nothing."),
		ro("ages", engine.JSON, "The minutes after which an open alert is first checked, nudged, carded and escalated, by severity."),
		ro("defaults", engine.Bool, "True when the values are the platform's defaults, not a choice of the organization."),
		ro("demoted", engine.JSON, "The kinds of alert that are only being suggested for now, each with the two alerts behind it."),
		ro("informative_kinds", engine.JSON, "The informative kinds that are proposed or approved, as data.sreagent_alert_informative_kinds answers them."),
		ro("updated_at", engine.String, "When the setting last changed, or null while it holds the defaults."),
		ro("week", engine.JSON, "What the follow-up did in the last 7 days, with the nudges, cards and escalations."),
	},
}

// AlertInformativeKinds is data.sreagent_alert_informative_kinds: the kinds of
// alert marked expected, which the follow-up closes by age or when their source
// clears. A person proposes one from an alert and an organization admin decides
// it, so Terraform reads them and never writes them.
var AlertInformativeKinds = engine.Spec{
	Key: "alert_informative_kinds", TypeName: "alert_informative_kind", ListName: "alert_informative_kinds",
	Description: "The informative kinds of alert (kinds marked expected): by default the proposals and the approved kinds, or the ones with one status, or all, newest first, at most 220. A person proposes a kind from an alert and an organization admin decides it in the app (or over MCP); Terraform only reads them.",
	Shape:       engine.Generated, View: true,
	Attrs: []engine.Attr{
		ro("label", engine.String, "What the kind is called."),
		ro("kind_key", engine.String, "The key the follow-up matches alerts of this kind by."),
		ro("source", engine.String, "The alert source the kind belongs to."),
		ro("matchers", engine.JSON, "What an alert must match to be of this kind: service, environment and one of cluster, namespace or data source."),
		ro("status", engine.String, "proposed, approved, declined or retired."),
		ro("action", engine.String, "What the follow-up does with an alert of this kind: resolve_when_cleared or expire_after."),
		ro("expire_after_hours", engine.Int, "The age in hours an expire_after kind closes its alerts after."),
		ro("expire_after_allowed", engine.Bool, "False for a kind of Grafana or CloudWatch alarms, which close when their source clears and never by age."),
		ro("proposed_by", engine.String, "Who proposed it (a former member reads as such), or null."),
		ro("proposed_at", engine.String, "When it was proposed, or null."),
		ro("decided_by", engine.String, "Who approved or declined it, or null."),
		ro("decided_at", engine.String, "When it was decided, or null."),
		ro("retired_reason", engine.String, "Why it was retired, or null."),
		ro("version", engine.String, "The version of the kind, which a decision passes back as based_on."),
		ro("open_alerts", engine.Int, "How many open alerts a proposed or approved kind would cover now; 0 for a declined or retired kind."),
	},
	QueryAttrs: []engine.Attr{
		{Name: "status", Kind: engine.String, OneOf: []string{"proposed", "approved", "declined", "retired", "all"}, Description: "One status, or all. Default: proposed and approved."},
	},
	ExtraAttrs: []engine.Attr{
		{Name: "counts", Kind: engine.JSON, Computed: true, Description: "The organization's proposed and approved kinds."},
		{Name: "limits", Kind: engine.JSON, Computed: true, Description: "How many kinds the organization may hold."},
	},
}
