resource "sreagent_service_binding" "checkout" {
  service           = "checkout"
  log_groups        = ["/aws/lambda/checkout"]
  log_inline_fields = ["trace_id", "user.id"]
}
