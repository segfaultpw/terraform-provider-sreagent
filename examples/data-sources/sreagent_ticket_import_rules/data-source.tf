data "sreagent_ticket_import_rules" "all" {}

output "ticket_import_rules_count" {
  value = length(data.sreagent_ticket_import_rules.all.items)
}
