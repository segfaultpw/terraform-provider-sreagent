data "sreagent_outbound_configs" "all" {}

output "outbound_configs_count" {
  value = length(data.sreagent_outbound_configs.all.items)
}
