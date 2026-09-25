data "sreagent_synthetic_checks" "all" {}

output "synthetic_checks_count" {
  value = length(data.sreagent_synthetic_checks.all.items)
}
