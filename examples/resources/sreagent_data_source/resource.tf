variable "prometheus_token" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "sreagent_data_source" "prometheus" {
  name      = "prometheus"
  type      = "prometheus"
  url       = "https://prometheus.example.com"
  auth_type = "bearer"
  auth_credentials_wo = jsonencode({
    token = var.prometheus_token
  })
  auth_credentials_wo_version = 1
}
