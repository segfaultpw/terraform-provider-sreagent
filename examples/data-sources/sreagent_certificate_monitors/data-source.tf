data "sreagent_certificate_monitors" "all" {}

output "certificate_monitors_count" {
  value = length(data.sreagent_certificate_monitors.all.items)
}
