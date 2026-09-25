data "sreagent_connectors" "all" {}

output "connectors_count" {
  value = length(data.sreagent_connectors.all.items)
}
