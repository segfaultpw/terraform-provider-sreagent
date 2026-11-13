# Days a finding may stay active before it breaches. A day left out is left as it is;
# 0 means no SLA for that severity.
resource "sreagent_security_sla_policy" "this" {
  critical_days = 3
  high_days     = 14
  medium_days   = 60
  low_days      = 180
  info_days     = 0
}
