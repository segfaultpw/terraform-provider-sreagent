resource "sreagent_certificate_monitor" "api" {
  hostname         = "api.example.com"
  warn_days_before = 21
}
