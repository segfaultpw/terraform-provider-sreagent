data "sreagent_ticket_integrations" "all" {}

output "ticket_integrations_count" {
  value = length(data.sreagent_ticket_integrations.all.items)
}
