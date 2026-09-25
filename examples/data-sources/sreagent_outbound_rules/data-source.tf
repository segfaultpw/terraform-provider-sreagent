data "sreagent_outbound_rules" "all" {}

output "outbound_rules_count" {
  value = length(data.sreagent_outbound_rules.all.items)
}
