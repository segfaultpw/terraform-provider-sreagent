# The days each severity may stay active before its SLA breaches; null means no SLA.
data "sreagent_security_sla_policy" "this" {}

output "critical_sla_days" {
  value = data.sreagent_security_sla_policy.this.critical_days
}
