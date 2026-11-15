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

  # Whether board tiles show a card's most used reactions. On by default; each
  # person can choose otherwise on their account page, and their choice wins.
  board_tile_reactions = false

  # How many hours the board shows who is looking at a card after they put eyes
  # on it: 4, 8, 24 (the default) or 72. 0 shows them until they remove the
  # reaction (the platform stores that as null, which has no number).
  looking_window_hours = 72

  # The reaction meanings switched off, as the whole set: looking (eyes on a card
  # makes the person a watcher and shows their face on its tile) and take_it (a
  # raised hand on a card nobody is assigned to assigns it to them). [] leaves
  # both on.
  reaction_meanings_off = ["take_it"]
}
