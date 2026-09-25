resource "sreagent_notification_settings" "this" {
  enabled    = true
  recipients = ["oncall@example.com"]
  notify_slo = true
}
