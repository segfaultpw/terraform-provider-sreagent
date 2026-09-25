data "sreagent_slis" "all" {}

output "slis_count" {
  value = length(data.sreagent_slis.all.items)
}
