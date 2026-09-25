variable "pagerduty_routing_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

# The routing key is write-only: it is sent when routing_key_wo_version
# changes and is never stored in state. Bump the version to rotate it.
resource "sreagent_outbound_config" "pagerduty" {
  name                   = "pagerduty"
  provider_type          = "pagerduty"
  routing_key_wo         = var.pagerduty_routing_key
  routing_key_wo_version = 1
}
