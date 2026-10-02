resource "sreagent_overseer_settings" "this" {
  enabled                       = true
  cadence                       = "daily"
  aggressivity                  = "conservative"
  digest_enabled                = true
  auto_ticket_critical_findings = true
}
