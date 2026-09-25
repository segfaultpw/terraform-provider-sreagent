resource "sreagent_sli" "latency" {
  name     = "checkout latency"
  sli_type = "latency"
  service  = "checkout"
}

resource "sreagent_slo" "latency" {
  name        = "checkout latency"
  sli_id      = sreagent_sli.latency.id
  target      = 99.9
  window_days = 30
}
