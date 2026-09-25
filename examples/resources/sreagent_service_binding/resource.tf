resource "sreagent_service_binding" "checkout_prod" {
  service     = "checkout"
  environment = "prod"
  log_groups  = ["/aws/lambda/checkout"]
}
