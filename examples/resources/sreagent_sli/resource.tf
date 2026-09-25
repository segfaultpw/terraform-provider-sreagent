resource "sreagent_sli" "latency" {
  name     = "checkout latency"
  sli_type = "latency"
  service  = "checkout"
}
