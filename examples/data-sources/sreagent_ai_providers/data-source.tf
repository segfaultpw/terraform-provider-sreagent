data "sreagent_ai_providers" "all" {}

output "ai_providers_count" {
  value = length(data.sreagent_ai_providers.all.items)
}
