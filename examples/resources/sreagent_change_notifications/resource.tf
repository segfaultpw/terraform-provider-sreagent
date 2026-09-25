# Needs the organization's own Slack workspace first.
resource "sreagent_change_notifications" "this" {
  enabled     = true
  channel     = "#deploys"
  event_types = ["deploy", "rollback"]
}
