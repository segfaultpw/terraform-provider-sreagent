resource "sreagent_organization_settings" "this" {
  alert_storm_threshold = 25
  timezone              = "America/Sao_Paulo"

  # The outcome lines SRE Agent writes into a card's Slack threads: en or es.
  language = "es"

  # The events NOT posted in those threads, as the whole set ([] posts
  # everything). A failed or declined fix or change is always posted.
  slack_thread_events_off = ["column_moves", "comments"]

  # The emoji shown first when someone adds a reaction, in order, written as the
  # emoji itself (never a :name:). [] uses SRE Agent's twelve.
  reaction_quick_row = ["\u2705", "\U0001F44D", "\U0001F680", "\U0001F440"]
}
