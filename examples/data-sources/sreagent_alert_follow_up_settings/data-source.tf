# Read only: an organization admin switches the alert follow-up on, and changes
# it, in Settings. It is off until then.
data "sreagent_alert_follow_up_settings" "this" {}

output "alert_follow_up_enabled" {
  value = data.sreagent_alert_follow_up_settings.this.enabled
}

output "critical_alerts_cleared_by_their_source" {
  value = jsondecode(data.sreagent_alert_follow_up_settings.this.resolve_modes).critical
}

# The three switches an organization admin turns on in Settings, each off until then.
output "alert_follow_up_switches" {
  value = {
    telemetry_checks     = data.sreagent_alert_follow_up_settings.this.telemetry_checks_enabled
    telemetry_model      = data.sreagent_alert_follow_up_settings.this.telemetry_model_enabled
    informative_learning = data.sreagent_alert_follow_up_settings.this.informative_learning_enabled
  }
}
