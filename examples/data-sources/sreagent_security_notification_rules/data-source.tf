data "sreagent_security_notification_rules" "all" {}

output "paging_rules" {
  value = [for r in data.sreagent_security_notification_rules.all.items : r.name if r.page]
}
