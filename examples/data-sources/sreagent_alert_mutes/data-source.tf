data "sreagent_alert_mutes" "all" {}

output "alert_mutes_count" {
  value = length(data.sreagent_alert_mutes.all.items)
}
