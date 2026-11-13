# Page on call for every new critical finding, and post it to a channel.
resource "sreagent_security_notification_rule" "critical" {
  name          = "Critical findings"
  min_severity  = "critical"
  events        = ["new_finding", "sla_breached"]
  slack_channel = "#security-alerts"
  email         = false
  page          = true
}
