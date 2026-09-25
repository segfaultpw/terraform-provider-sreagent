data "sreagent_status_page_components" "all" {}

output "status_page_components_count" {
  value = length(data.sreagent_status_page_components.all.items)
}
