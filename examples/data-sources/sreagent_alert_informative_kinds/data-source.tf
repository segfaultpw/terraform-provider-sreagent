# The kinds of alert marked expected. By default the proposals and the approved
# kinds; status = "all" also lists the declined and retired ones.
data "sreagent_alert_informative_kinds" "approved" {
  status = "approved"
}

output "approved_kinds" {
  value = [for k in data.sreagent_alert_informative_kinds.approved.items : k.label]
}
