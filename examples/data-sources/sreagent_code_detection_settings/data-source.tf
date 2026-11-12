# Read only: the weekly code detection schedule is switched on in the app.
data "sreagent_code_detection_settings" "this" {}

output "weekly_code_detection" {
  value = data.sreagent_code_detection_settings.this.weekly_enabled
}
