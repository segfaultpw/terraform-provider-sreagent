variable "ssh_private_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

# The whole config is stored encrypted, so it is write-only as a whole.
resource "sreagent_connector" "bastion" {
  name           = "bastion"
  connector_type = "ssh"
  config_wo = jsonencode({
    host        = "10.0.0.10"
    username    = "sre"
    private_key = var.ssh_private_key
  })
  config_wo_version = 1
}
