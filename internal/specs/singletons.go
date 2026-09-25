package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// singleton builds a one-row-per-organization spec: create adopts the
// current row, destroy leaves it in place.
func singleton(key, desc string, attrs ...engine.Attr) engine.Spec {
	return engine.Spec{Key: key, TypeName: key, ListName: key, Description: desc + " One row per organization: creating it adopts the current settings, destroying it leaves them in place.", Shape: engine.Singleton, Lifecycle: true, Attrs: attrs}
}

func b(name, desc string) engine.Attr {
	return engine.Attr{Name: name, Kind: engine.Bool, Description: desc}
}
func i(name, desc string) engine.Attr {
	return engine.Attr{Name: name, Kind: engine.Int, Description: desc}
}
func s(name, desc string) engine.Attr {
	return engine.Attr{Name: name, Kind: engine.String, Description: desc}
}
func l(name, desc string) engine.Attr {
	return engine.Attr{Name: name, Kind: engine.StringList, Description: desc}
}
func computed(a engine.Attr) engine.Attr { a.Computed = true; return a }

// OrganizationSettings mirrors x-sreagent-resources.organization_settings.
var OrganizationSettings = singleton("organization_settings", "The organization's own settings.",
	i("alert_storm_threshold", "Alerts within the window that make a storm."),
	i("alert_storm_window_seconds", "The storm window."),
	b("alert_grouping_ai_enabled", "Group related alerts with AI."),
	i("alert_refire_cooldown_minutes", "0 to 1440."),
	i("automation_approval_ttl_minutes", "How long an approval stays valid."),
	i("investigation_cooldown_minutes", "Minutes between investigations of one alert."),
	b("auto_open_incidents", "Open incidents automatically."),
	b("auto_close_incidents", "Close incidents automatically."),
	s("incident_severity_floor", "The lowest severity that opens an incident."),
	b("investigation_paused", "Pause automatic investigations."),
	i("monthly_ai_budget_usd", "The monthly AI budget."),
	i("ai_budget_warn_percent", "Warn at this share of the budget."),
	// A person decides these two: the platform refuses them from any
	// connection without a user identity, which an API key never has.
	computed(b("social_joins_enabled", "Allow joining through social sign-in. Changed by an admin in the app.")),
	computed(b("restrict_domain_signups", "Only allow sign-ups from the organization's domains. Changed by an admin in the app.")),
	s("timezone", "An IANA time zone."),
)

// NotificationSettings mirrors x-sreagent-resources.notification_settings.
var NotificationSettings = singleton("notification_settings", "Email notification settings.",
	b("enabled", "Send notification emails."),
	l("recipients", "Who receives them."),
	b("notify_incidents", "Email on incidents."),
	b("notify_slo", "Email on SLO events."),
	b("notify_automation", "Email on automation events."),
	b("notify_security", "Email on security events."),
	l("muted_events", "Event types never emailed."),
)

// ChangeNotifications mirrors x-sreagent-resources.change_notifications. A
// child organization reading its parent's row sees no row of its own.
var ChangeNotifications = func() engine.Spec {
	sp := singleton("change_notifications", "Where deploy and config change announcements post in Slack. Needs a Slack workspace of this organization's own (sreagent_slack) first.",
		b("enabled", "Announce changes."),
		s("channel", "The channel."),
		l("event_types", "deploy, scale, restart, config_change, rollback."),
		s("namespace_filter", "Only this Kubernetes namespace."),
	)
	sp.StartsEmpty = true
	return sp
}()

// Slack mirrors x-sreagent-resources.slack. The workspace is verified
// against Slack with the bot token, so it starts with no row.
var Slack = func() engine.Spec {
	sp := singleton("slack", "The organization's Slack workspace.",
		s("name", "A label for the workspace; required on first configuration."),
		s("workspace_name", "The workspace's name."),
		s("team_id", "The T... workspace id, verified against Slack with the bot token."),
		b("enabled", "Whether the workspace is used."),
		s("default_channel_critical", "Where critical alerts post when no route names a channel."),
		s("default_channel_warning", "Where warning alerts post when no route names a channel."),
		s("default_channel_info", "Where info alerts post when no route names a channel."),
		s("default_channel_remediation", "Where remediation updates post."),
		engine.Attr{Name: "bot_token", Kind: engine.String, Secret: true, Description: "The bot token."},
		engine.Attr{Name: "signing_secret", Kind: engine.String, Secret: true, Description: "The signing secret."},
		computed(i("priority", "Order among workspaces.")),
		computed(b("app_token_set", "Whether a Socket Mode app token is stored.")),
	)
	sp.StartsEmpty = true
	return sp
}()

// AISettings mirrors x-sreagent-resources.ai_settings. retry_budget answers
// the budget in force, not only a stored one.
var AISettings = singleton("ai_settings", "Organization-wide AI settings.",
	i("retry_budget", "Retries on transient provider errors; answers the budget in force."),
	b("own_providers_only", "Route only to the organization's own providers."),
	b("anonymization_enabled", "Anonymize data sent to models."),
	b("multi_agent_investigation", "Use multi-agent investigations."),
	computed(b("retry_budget_configured", "Whether retry_budget was set rather than defaulted.")),
)

// OverseerSettings mirrors x-sreagent-resources.overseer_settings.
var OverseerSettings = singleton("overseer_settings", "The Overseer's schedule and budget.",
	b("enabled", "Run the Overseer."),
	s("cadence", "How often it runs."),
	s("aggressivity", "How much it proposes."),
	i("max_tokens_per_run", "Token budget per run."),
	i("max_tool_calls", "Tool call budget per run."),
	b("digest_enabled", "Send a digest."),
	computed(s("next_run_at", "When it runs next.")),
	computed(s("last_skip_reason", "Why the last due run did nothing.")),
)

// GitHubSettings mirrors x-sreagent-resources.github_settings. Every
// organization reads a row; writing one needs a connected installation.
var GitHubSettings = singleton("github_settings", "The GitHub App installation's settings. Needs a connected installation.",
	b("auto_pr_enabled", "Open fix PRs automatically."),
	b("auto_revise_on_review", "Revise a PR when a review asks."),
	b("draft_prs", "Open PRs as drafts."),
	b("review_enabled", "Review pull requests."),
	b("link_comments_enabled", "Comment links on linked issues."),
	engine.Attr{Name: "review_severity_floor", Kind: engine.String, OneOf: []string{"critical", "high", "medium", "low"}, Description: "The lowest severity a review comments on."},
	s("context_repo", "A repository whose files give reviews context."),
	computed(b("connected", "Whether an installation is connected.")),
	computed(s("account", "The installation's account.")),
	computed(i("installation_id", "The installation id.")),
)

// StatusPageSettings mirrors x-sreagent-resources.status_page_settings.
var StatusPageSettings = singleton("status_page_settings", "The public status page's settings. Branding fields need the custom branding plan feature.",
	b("enabled", "Publish the page."),
	s("title", "The page title."),
	s("page_description", "Shown under the title."),
	s("logo_url", "The logo."),
	s("primary_color", "The accent colour."),
	s("support_url", "Where the support link points."),
	i("show_history_days", "7 to 90."),
	s("timezone", "An IANA time zone."),
	b("subscribe_enabled", "Allow email subscriptions."),
	b("show_uptime_summary", "Show uptime percentages."),
	computed(b("configured", "Whether the page has been saved.")),
	computed(i("active_incidents", "Incidents in progress.")),
)
