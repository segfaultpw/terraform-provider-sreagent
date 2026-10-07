resource "sreagent_outbound_config" "hook" {
  name          = "incident-webhook"
  provider_type = "webhook"
  # A webhook URL carries its own token, so it is write-only: never stored in state.
  base_url_wo         = "https://hooks.example.com/incidents"
  base_url_wo_version = 1
}

resource "sreagent_outbound_rule" "sev1" {
  name                   = "sev1"
  outbound_config_id     = sreagent_outbound_config.hook.id
  match_severity         = "critical"
  escalate_after_minutes = 10
}
