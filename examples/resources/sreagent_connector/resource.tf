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

# The connector self-healing's Security area deactivates an unused IAM access key through.
# Its role comes from the terraform-sre-agent module aws-iam-hygiene. No runbook step can use it.
resource "sreagent_connector" "iam_hygiene" {
  name           = "iam hygiene"
  connector_type = "aws_iam"
  config_wo = jsonencode({
    auth_type = "aws_assume_role"
    role_arn  = "arn:aws:iam::123456789012:role/sre-agent-iam-hygiene"
    region    = "us-east-1"
  })
  config_wo_version = 1
}
