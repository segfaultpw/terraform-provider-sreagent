data "sreagent_status_page_incidents" "all" {}

output "status_page_incidents_count" {
  value = length(data.sreagent_status_page_incidents.all.items)
}
