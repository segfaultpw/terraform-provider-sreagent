data "sreagent_alert_routes" "all" {}

output "alert_routes_count" {
  value = length(data.sreagent_alert_routes.all.items)
}
