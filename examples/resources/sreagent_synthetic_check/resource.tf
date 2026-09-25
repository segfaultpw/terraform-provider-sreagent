# Request headers and a body are set in the app: the platform never answers
# them, so Terraform refuses them in config.
resource "sreagent_synthetic_check" "home" {
  name             = "home page"
  check_type       = "http"
  target           = "https://example.com"
  interval_seconds = 60
  config = jsonencode({
    method = "GET"
  })
}
