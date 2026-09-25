data "sreagent_service_bindings" "all" {}

output "service_bindings_count" {
  value = length(data.sreagent_service_bindings.all.items)
}
