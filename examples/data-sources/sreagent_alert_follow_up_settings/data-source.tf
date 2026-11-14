# Read only: an organization admin switches the alert follow-up on, and changes
# it, in Settings. It is off until then.
data "sreagent_alert_follow_up_settings" "this" {}

output "alert_follow_up_enabled" {
  value = data.sreagent_alert_follow_up_settings.this.enabled
}

output "critical_alerts_cleared_by_their_source" {
  value = jsondecode(data.sreagent_alert_follow_up_settings.this.resolve_modes).critical
}
