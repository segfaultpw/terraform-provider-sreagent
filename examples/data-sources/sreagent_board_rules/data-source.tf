# Read only: an organization admin writes the board's rules in Settings, Board rules.
data "sreagent_board_rules" "all" {}

output "enabled_rules" {
  value = [for r in data.sreagent_board_rules.all.items : r.name if r.enabled]
}

output "first_rule_actions" {
  value = length(data.sreagent_board_rules.all.items) > 0 ? jsondecode(data.sreagent_board_rules.all.items[0].actions) : []
}
