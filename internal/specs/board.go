package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// The board's templates, recurring cards and rules, and the lead agent's settings, are
// people's decisions made in the app (a member writes a template, an organization
// admin a schedule, rule or SRE Agent's doors), and the platform publishes no
// Terraform type for them, so the provider reads them as data sources only. Each
// becomes a resource when the platform types it.

// TicketTemplates is data.sreagent_ticket_templates: the card templates of the
// organization's board.
var TicketTemplates = engine.Spec{
	Key: "ticket_templates", TypeName: "ticket_template", ListName: "ticket_templates",
	Description: "The card templates of the organization's board: a saved starting point for a card (its title, description, labels, priority, team, service and checklist) that a member picks when making a card, that a card is saved as, and that a recurring card (data.sreagent_ticket_schedules) is made from. Read only: a member writes a template on the board's Templates page or over MCP, and the platform publishes no Terraform type for it. Needs platform v0.523.0.",
	Shape:       engine.Generated,
	Attrs: []engine.Attr{
		ro("name", engine.String, "The template's name, as the picker shows it."),
		ro("title", engine.String, "The title a card made from the template starts with."),
		ro("description", engine.String, "The description a card starts with, or null."),
		ro("labels", engine.StringList, "The labels a card starts with, or null."),
		ro("priority", engine.String, "The priority a card starts with, or null."),
		ro("team_id", engine.String, "The id of the team a card starts assigned to, or null."),
		ro("service", engine.String, "The service a card starts linked to, or null."),
		ro("checklist", engine.StringList, "The checklist items a card starts with, in order, or null."),
		ro("updated_at", engine.String, "When the template last changed."),
	},
}

// TicketSchedules is data.sreagent_ticket_schedules: the recurring cards of the
// organization's board.
var TicketSchedules = engine.Spec{
	Key: "ticket_schedules", TypeName: "ticket_schedule", ListName: "ticket_schedules",
	Description: "The recurring cards of the organization's board: each opens a card from a template on a daily, weekly or monthly rhythm, assigned to a person or to whoever is on call, with an optional due date. Read only: an organization admin writes a schedule on the board's Templates page or over MCP, and the platform publishes no Terraform type for it. Needs platform v0.523.0.",
	Shape:       engine.Generated,
	Attrs: []engine.Attr{
		ro("name", engine.String, "The schedule's name."),
		ro("template_id", engine.String, "The id of the template (data.sreagent_ticket_templates) each card is made from."),
		ro("rhythm", engine.String, "daily, weekly or monthly."),
		ro("weekdays", engine.JSON, "For a weekly rhythm, the days of the week it runs on, as numbers, or null."),
		ro("interval_weeks", engine.Int, "For a weekly rhythm, every how many weeks, or null."),
		ro("day_of_month", engine.Int, "For a monthly rhythm, the day of the month, or null."),
		ro("interval_months", engine.Int, "For a monthly rhythm, every how many months, or null."),
		ro("hour", engine.Int, "The hour of the day the card opens, in the organization's time zone."),
		ro("anchor_on", engine.String, "The date the rhythm counts from, or null."),
		ro("assignee_mode", engine.String, "none, person (assignee_user_id) or on_call (oncall_schedule_id)."),
		ro("assignee_user_id", engine.String, "The person each card is assigned to, or null."),
		ro("oncall_schedule_id", engine.String, "The on-call schedule whose current person each card is assigned to, or null."),
		ro("due_in_days", engine.Int, "How many days after it opens a card is due, or null for no due date."),
		ro("enabled", engine.Bool, "Whether the schedule opens cards. The platform pauses a schedule whose runs keep failing (paused_reason says why)."),
		ro("description", engine.String, "The rhythm in words, as the Templates page shows it."),
		ro("last_run", engine.JSON, "The latest run (when, what happened and the card it opened), or null."),
		ro("last_run_at", engine.String, "When the schedule last ran, or null."),
		ro("next_run_at", engine.String, "When the schedule runs next, or null while it is paused or off."),
		ro("paused_reason", engine.String, "Why the platform paused the schedule, or null."),
		ro("updated_at", engine.String, "When the schedule last changed."),
	},
}

// BoardRules is data.sreagent_board_rules: the board's automation rules.
var BoardRules = engine.Spec{
	Key: "board_rules", TypeName: "board_rule", ListName: "board_rules",
	Description: "The board's rules: each runs actions on a card when something happens to it (a trigger such as a card being created or moved, or sitting in a column or past a due date), when its conditions hold. Read only: an organization admin writes a rule on Settings, Board rules or over MCP, and the platform publishes no Terraform type for it. A rule's `trigger`, `conditions` and `actions` are JSON; the condition kinds are service_is, has_label, lacks_label, team_is, priority_is, source_is, column_is and assignees, and the action kinds are assign, assign_on_call, set_priority, add_label, remove_label, move, comment, remind, add_checklist, add_checklist_template and assign_to_agent. Needs platform v0.525.0.",
	Shape:       engine.Generated,
	Attrs: []engine.Attr{
		ro("name", engine.String, "The rule's name."),
		ro("enabled", engine.Bool, "Whether the rule runs."),
		ro("trigger", engine.JSON, "What starts the rule: a kind and its arguments."),
		ro("conditions", engine.JSON, "The conditions that must all hold, each a kind and its arguments, or null."),
		ro("actions", engine.JSON, "What the rule does, in order, each a kind and its arguments."),
		ro("version", engine.Int, "How many times the rule was saved."),
		ro("last_run_at", engine.String, "When the rule last ran, or null."),
		ro("updated_at", engine.String, "When the rule last changed."),
	},
}

// LeadAgentSettings is data.sreagent_lead_agent_settings: what SRE Agent may do
// when it is given work on a card, in Slack or on an alert.
var LeadAgentSettings = readSingleton("lead_agent_settings",
	"What SRE Agent does when it is given work, as Settings, SRE Agent on cards shows it: for each place work comes from (a card, a Slack thread, an alert) whether it does nothing (off), plans and waits for a member to accept or run a step (suggest) or acts (act), whether a plan needs a member's acceptance, how much one goal may spend (wakes, tokens and hours), and for alerts whether it may page, act without a person, the lowest severity it takes and how many alerts it starts an hour. Read only: an organization admin changes it in Settings (or with the MCP tool update_lead_agent_settings), and the platform publishes no Terraform type for it. A place that has not been switched on is off or suggest by default, and defaults says whether these are the platform's. Needs platform v0.524.0.",
	"v0.524.0 or later", "",
	ro("card_mode", engine.String, "What SRE Agent does with a card given to it: off, suggest or act."),
	ro("slack_mode", engine.String, "What it does with a request made in a Slack thread: off, suggest or act."),
	ro("alert_mode", engine.String, "What it does with a new alert: off, suggest (it plans in the background and the automatic investigation runs as before) or act."),
	ro("plan_approval", engine.Bool, "Whether a plan needs a member's acceptance before anything is delegated."),
	ro("max_wakes", engine.Int, "The most steps one goal may take."),
	ro("max_tokens", engine.Int, "The most model tokens one goal may count."),
	ro("max_hours", engine.Int, "How long one goal may stay open, in hours."),
	ro("alert_paging", engine.Bool, "Whether SRE Agent may page the person on call for a critical or high alert that still fires."),
	ro("alert_act_unattended", engine.Bool, "Whether SRE Agent may request a fix or run a runbook for an alert without a person, within what self-healing allows."),
	ro("alert_min_severity", engine.String, "The lowest severity of alert it takes: critical, high or medium."),
	ro("alert_starts_per_hour", engine.Int, "How many alerts it starts on in an hour, 1 to 10."),
	ro("bounds", engine.JSON, "The limits each of max_wakes, max_tokens and max_hours may be set within."),
	ro("defaults", engine.Bool, "True while the organization has not saved a choice and the platform's defaults apply."),
	ro("modes", engine.JSON, "The modes a place may be set to."),
	ro("updated_at", engine.String, "When the settings last changed, or null while they hold the defaults."),
)
