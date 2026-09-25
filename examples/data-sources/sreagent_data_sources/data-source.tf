data "sreagent_data_sources" "all" {}

output "data_sources_count" {
  value = length(data.sreagent_data_sources.all.items)
}
