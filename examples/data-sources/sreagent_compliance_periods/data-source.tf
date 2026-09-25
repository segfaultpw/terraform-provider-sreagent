data "sreagent_compliance_periods" "all" {}

output "compliance_periods_count" {
  value = length(data.sreagent_compliance_periods.all.items)
}
