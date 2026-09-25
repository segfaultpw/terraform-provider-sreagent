resource "sreagent_overseer_settings" "this" {
  enabled        = true
  cadence        = "daily"
  aggressivity   = "conservative"
  digest_enabled = true
}
