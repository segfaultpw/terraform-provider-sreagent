# A mute cannot be edited: any change replaces it.
resource "sreagent_alert_mute" "storage_migration" {
  pattern          = "disk-full"
  reason           = "Planned storage migration"
  duration_minutes = 120
}
