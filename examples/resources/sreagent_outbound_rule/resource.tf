resource "sreagent_outbound_config" "hook" {
  name          = "incident-webhook"
  provider_type = "webhook"
  base_url      = "https://hooks.example.com/incidents"
}

resource "sreagent_outbound_rule" "sev1" {
  name                   = "sev1"
  outbound_config_id     = sreagent_outbound_config.hook.id
  match_severity         = "critical"
  escalate_after_minutes = 10
}
