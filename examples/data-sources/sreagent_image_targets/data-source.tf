data "sreagent_image_targets" "all" {}

output "image_targets_count" {
  value = length(data.sreagent_image_targets.all.items)
}
