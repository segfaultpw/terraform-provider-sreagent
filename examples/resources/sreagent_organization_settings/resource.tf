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
  # makes the person a watcher and shows their face on its tile), take_it (a
  # raised hand on a card nobody is assigned to assigns it to them), tell_author
  # (people are told when others react to their cards and comments) and
  # answer_feedback (a thumbs on an assistant answer is recorded as a rating).
  # [] leaves all four on.
  reaction_meanings_off = ["take_it", "answer_feedback"]

  # The emoji shown before each board column's name, as the whole set: a map from
  # a column (backlog, triage, todo, in_progress, blocked, done) to one emoji. A
  # column left out has no emoji, and {} removes them all.
  board_column_emoji = {
    in_progress = "\U0001F680"
    done        = "\u2705"
  }

  # How reactions travel between a card and its Slack threads: both (the
  # default), from_slack (a reaction in Slack shows on the card and SRE Agent adds
  # none) or off.
  slack_reaction_sync = "from_slack"

  # Whether SRE Agent's messages in a card's Slack thread carry the board's
  # counts. On by default.
  slack_reaction_counts = false
}
