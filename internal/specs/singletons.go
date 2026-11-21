package specs

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// singleton builds a one-row-per-organization spec: create adopts the
// current row, destroy leaves it in place.
func singleton(key, desc string, attrs ...engine.Attr) engine.Spec {
	return engine.Spec{Key: key, TypeName: key, ListName: key, Description: desc + " One row per organization: creating it adopts the current settings, destroying it leaves them in place.", Shape: engine.Singleton, Lifecycle: true, Attrs: attrs}
}

// withDestroyNote says what stays in force when the spec's destroy leaves the settings.
func withDestroyNote(sp engine.Spec, note string) engine.Spec {
	sp.DestroyNote = note
	return sp
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

// reactionMeanings are the meanings of a reaction an organization can switch off, in the order
// the platform lists them.
var reactionMeanings = []string{"looking", "take_it", "tell_author", "answer_feedback"}

// boardColumns are the board columns that can carry an emoji, in board order.
var boardColumns = []string{"backlog", "triage", "todo", "in_progress", "blocked", "done"}

// slackReactionSync are the ways reactions travel between a card and its Slack threads.
var slackReactionSync = []string{"both", "from_slack", "off"}

// threadEvents are the events a card's Slack threads can be told not to post,
// in the order the platform lists them. A failed, declined or cancelled fix or
// change is always posted and has no name here.
var threadEvents = []string{"column_moves", "card_done", "card_wont_do", "pull_request_opened", "pull_request_merged", "pull_request_closed", "pull_request_reopened", "change_pushed", "assistant_replies", "comments"}

// emojiForm is the form in which two spellings of one emoji are one value: the
// platform removes the text and emoji presentation selectors (U+FE0E, U+FE0F),
// looks the bare form up and stores the fully qualified one, so a configuration
// that omits or adds a selector is the stored emoji.
func emojiForm(v string) string {
	return strings.NewReplacer("\ufe0e", "", "\ufe0f", "").Replace(v)
}

// slackName refuses an entry written as a Slack name in colons: the platform
// accepts it and stores the emoji it names, which Terraform could not tell from a
// different value, so the entry would plan a change on every run.
func slackName(entry string) string {
	if len(entry) > 1 && strings.HasPrefix(entry, ":") && strings.HasSuffix(entry, ":") {
		return fmt.Sprintf("%q is a Slack name. The platform accepts it and stores the emoji it names, so it would read back as a different value and plan a change on every run. Write the emoji itself.", entry)
	}
	return ""
}

// The most one reaction emoji may hold. SreData.Emoji.normalize/1 of the platform
// refuses a value over max_code_points/0 (16) or max_bytes/0 (64, UTF-8), so a
// longer entry can never be stored; the published items.maxLength of 64 is the
// looser published bound and is checked separately.
const (
	emojiMaxCodePoints = 16
	emojiMaxBytes      = 64
)

// quickRowEntry refuses an entry the platform would refuse (more than 16 code
// points or 64 bytes) or store differently than written (a Slack name).
func quickRowEntry(entry string) string {
	// A Slack name is judged first: a long one (:white_check_mark:) is over the bound too, and the
	// useful thing to tell the person is to write the emoji.
	if sentence := slackName(entry); sentence != "" {
		return sentence
	}
	if n := utf8.RuneCountInString(entry); n > emojiMaxCodePoints || len(entry) > emojiMaxBytes {
		return fmt.Sprintf("An emoji holds at most %d code points and %d bytes, as the platform counts them; this entry has %d code points and %d bytes.", emojiMaxCodePoints, emojiMaxBytes, n, len(entry))
	}
	return ""
}

// OrganizationSettings mirrors x-sreagent-resources.organization_settings.
var OrganizationSettings = withDestroyNote(singleton("organization_settings", "The organization's own settings.",
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
	// The platform lists fix_request_floor as terraform_read_only: only an
	// org admin's own session changes it, so it is read here, never written.
	computed(engine.Attr{Name: "fix_request_floor", Kind: engine.String, OneOf: []string{"org_admin", "member"}, Description: "Who may ask SRE Agent for a fix pull request, or to change one it opened: org_admin for org admins only, member for members too. Changed by an org admin in the app."}),
	// The sign-in and key limits are decided by an org admin in the app (or by
	// the person it names), so the provider reads them and never writes them.
	computed(b("mfa_required", "Whether the organization requires two-step sign-in. Changed by an org admin in the app.")),
	computed(i("mfa_grace_days", "Days a person has to enrol a second step once it is required, 0 to 30. Changed by an org admin in the app.")),
	computed(i("session_idle_minutes", "Minutes of inactivity after which a browser session is signed out, 5 to 43200, or null for no limit. Changed by an org admin in the app.")),
	computed(i("api_key_max_days", "The longest life, in days, a new API key or personal token may be given, or null for no limit. Changed by an org admin in the app.")),
	computed(engine.Attr{Name: "mfa_effective", Kind: engine.JSON, Description: "The two-step rule that applies to this organization, counting the strictest of its own and its parent's settings."}),
	computed(i("session_idle_effective", "The idle sign-out limit that applies to this organization, in minutes, counting its parent's, or null for none.")),
	engine.Attr{Name: "inheritance_stopped", Kind: engine.StringList,
		IgnoredValues: []string{"repository_defaults"},
		IgnoredWhy:    "A child gives itself its own repository defaults with sreagent_repository_defaults (destroy it to follow the parent again), or stops following its parent's in the app (it then reads the built-in values); this list never changes either.",
		Description:   "The connections of the parent organization this organization does not use, as the whole set: slack, github_installation, ai_providers, outbound_targets, ticket_integrations. Only a child organization can stop using a parent's. The platform refuses a change of the list from an API key, so a value that differs from the one it holds fails the apply; set it in the app. The platform also keeps a sixth kind, repository_defaults, which this list never reads, sends or accepts: it is changed in the app, and sreagent_repository_defaults gives a child its own defaults instead."},
	engine.Attr{Name: "language", Kind: engine.String, OneOf: []string{"en", "es"}, Description: "The language of the outcome lines SRE Agent writes into a card's Slack threads: how a fix, a change or a pull request ended, a card moved to Done or closed as won't do, and the label in front of a comment posted from the card. en (English, the default) or es (Spanish). Names, links, error messages and what people and the AI wrote are posted as they are. The card itself, its comments, the app's pages and emails stay in English. An organization admin's key (api:admin) may change it."},
	engine.Attr{Name: "slack_thread_events_off", Kind: engine.StringList, Unordered: true, OneOf: threadEvents, Description: "The events NOT posted in a card's Slack threads, as the whole set: [] posts everything (the default), and a list posts everything else. The order does not matter. Failed, declined or cancelled fixes and changes, a failed local fix session, a change that could not be made on the pull request it was asked for and the assistant's answer to a question asked in Slack are always posted and have no name here. The card, the tracker and the notices to the card's people are not affected."},
	engine.Attr{Name: "reaction_quick_row", Kind: engine.StringList, MaxItems: 12, ItemMaxLength: 64, Normalize: emojiForm, ItemCheck: quickRowEntry, Description: "The emoji shown first when someone adds a reaction, in order, as the whole list: up to 12, each one emoji (the platform refuses an entry over 16 code points or 64 bytes) written as the emoji itself, never repeated (the same emoji spelled with and without its presentation selector counts as a repeat). [] uses SRE Agent's twelve. The platform stores the fully qualified form of each emoji; an entry written without a selector (for example a heart or a thumbs up spelled as one code point) is accepted, kept as you wrote it in state and plans no change. A Slack name such as :tada: is accepted by the platform but stores the emoji it names, so it is refused here instead of planning a change on every run. An organization admin's key (api:admin) may change it."},
	b("board_tile_reactions", "Whether board tiles show a card's three most used reactions with their counts. On by default; each person can choose otherwise on their account page, and their choice wins. An organization admin's key (api:admin) may change it."),
	engine.Attr{Name: "looking_window_hours", Kind: engine.Int, IntOneOf: []int64{4, 8, 24, 72}, NullAs: "0", NullAsMeaning: "to show who is looking until the reaction is removed",
		Description:     "How many hours the board shows who is looking at a card after they put eyes on it: 4, 8, 24 (the default) or 72, or 0 to show them until they remove the reaction. The platform stores \"until they remove the reaction\" as null, which has no number, so this attribute writes it as 0 (the data source reads the null as null). Leaving the attribute out keeps whatever the organization has. An organization admin's key (api:admin) may change it.",
		ReadDescription: "How many hours the board shows who is looking at a card after they put eyes on it: 4, 8, 24 (the default) or 72, or null to show them until they remove the reaction. An organization admin's key (api:admin) may change it."},
	engine.Attr{Name: "reaction_meanings_off", Kind: engine.StringList, Unordered: true, OneOf: reactionMeanings, KeepUnknown: true, Description: "The reaction meanings switched off, as the whole set: [] leaves every meaning on (the default). looking switches off eyes on a card making the person a watcher and showing their face on its tile; take_it switches off a raised hand on a card nobody is assigned to assigning it to the person; tell_author switches off telling people, in the attention list and the morning briefing, when others react to their cards and comments (each person can also switch it off for themselves on their account page); answer_feedback switches off recording a thumbs up or down on an assistant answer or an investigation as feedback on it, so new ones are no longer recorded. The order does not matter. A reaction already made keeps its chip. This attribute lists only the meanings this provider release knows. A meaning a later platform release adds and an admin switched off in the app is left out of it, stays off on every apply (it is sent along with the set you write, never as removed), and is not shown as a change; read data.sreagent_organization_settings to see every value the platform holds. A plan of a block already in state that names this attribute warns, naming the value, once; the first plan that adopts the singleton does not. When you upgrade to a provider release that declares the value, add it to this attribute if it should stay off: the plan then shows it as removed, and the next apply switches it back on unless the configuration names it. An organization admin's key (api:admin) may change it.",
		ReadDescription: "The reaction meanings switched off, as the whole set: [] leaves every meaning on (the default). looking is eyes on a card making the person a watcher and showing their face on its tile; take_it is a raised hand on a card nobody is assigned to assigning it to the person; tell_author is telling people, in the attention list and the morning briefing, when others react to their cards and comments; answer_feedback is recording a thumbs up or down on an assistant answer or an investigation as feedback on it. The order does not matter. A reaction already made keeps its chip. The data source lists every meaning the platform holds, including one a later platform release adds that the resource does not manage. An organization admin's key (api:admin) may change it."},
	engine.Attr{Name: "board_column_emoji", Kind: engine.StringMap, MapKeys: boardColumns, ItemMaxLength: 64, Normalize: emojiForm, ItemCheck: quickRowEntry,
		Description: "The emoji shown before each board column's name, as the whole set: a map from a column (backlog, triage, todo, in_progress, blocked or done) to one emoji written as the emoji itself. A column left out has no emoji, so {} removes them all; the map replaces what the organization holds, so a column the platform has an emoji for and the configuration leaves out is removed. Display only: it changes what the board's column headers, the Move to menu, the card's status buttons and the new card form say. The platform stores the fully qualified form of each emoji; a value written without a selector (for example a heart spelled as one code point) is accepted, kept as you wrote it in state and plans no change. A Slack name such as :white_check_mark: is accepted by the platform but stores the emoji it names, so it is refused here instead of planning a change on every run, and so is a column with a null value (leave it out). An organization admin's key (api:admin) may change it."},
	engine.Attr{Name: "slack_reaction_sync", Kind: engine.String, OneOf: slackReactionSync, Description: "How reactions travel between a card and its Slack threads: both (the default), from_slack (a reaction in Slack shows on the card, and SRE Agent adds none in Slack) or off (nothing arrives from Slack, and SRE Agent adds none). Switching never removes reactions already in place. An organization admin's key (api:admin) may change it."},
	b("slack_reaction_counts", "Whether SRE Agent's messages in a card's Slack thread carry the board's counts (\"On the board: :+1: 2\"). On by default. An organization admin's key (api:admin) may change it."),
), "The language, the Slack thread events, the quick reactions, the tile reactions, the looking window, the reaction meanings, the column emoji and the Slack reaction settings stay as they are; to put them back to their defaults, state en, [], [], true, 24, [], {}, both and true, apply, then remove the block.")

// NotificationSettings mirrors x-sreagent-resources.notification_settings.
var NotificationSettings = singleton("notification_settings", "Email notification settings.",
	b("enabled", "Send notification emails."),
	l("recipients", "Who receives them."),
	b("notify_incidents", "Email on incidents."),
	b("notify_slo", "Email on SLO events."),
	b("notify_automation", "Email on automation events."),
	b("notify_security", "Email on security events."),
	l("muted_events", "Event types never emailed (for example security_digest, the weekly or daily security digest)."),
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
		computed(engine.Attr{Name: "delivery_problems", Kind: engine.JSON, Description: "The channels Slack refused this workspace's posts to, and a workspace-wide refusal, as the Settings page lists them."}),
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
	b("auto_ticket_critical_findings", "Open an urgent ticket when a critical finding newly enters critical. Needs a plan with the ticket board."),
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
	b("auto_merge_requires_verification", "Auto-merge only what SRE Agent verified. Off by default: a fix SRE Agent's own checks did not run for can still merge by itself once the checks its branch requires pass (where a repository's auto_merge says so), and a change pushed onto a pull request afterwards merges on those checks alone. An organization admin takes that risk by leaving it off. On: only a fix whose exact commit those checks ran on and passed merges by itself. In both positions a fix whose checks failed, or ran and could not judge it, or has an open self-review blocker, never merges by itself."),
	engine.Attr{Name: "review_severity_floor", Kind: engine.String, OneOf: []string{"critical", "high", "medium", "low"}, Description: "The lowest severity a review comments on."},
	s("context_repo", "A repository whose files give reviews context."),
	computed(b("connected", "Whether an installation is connected.")),
	computed(s("account", "The installation's account.")),
	computed(i("installation_id", "The installation id.")),
	computed(s("stopped_from", "The id of the parent organization whose GitHub App this organization stopped using, null when it has not stopped.")),
	computed(s("stopped_note", "Why the installation reads as not connected after the organization stopped using its parent's GitHub App; null otherwise. A child organization changes this in the app (inheritance_stopped).")),
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
