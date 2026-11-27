# Read only: a member writes card templates on the board's Templates page.
data "sreagent_ticket_templates" "all" {}

output "template_names" {
  value = [for t in data.sreagent_ticket_templates.all.items : t.name]
}
