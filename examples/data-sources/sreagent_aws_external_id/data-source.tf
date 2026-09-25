# The IAM role a data source assumes, created in the same apply as the data
# source: its trust policy needs the organization's ExternalId and the
# platform's principal.
data "sreagent_aws_external_id" "this" {}

resource "aws_iam_role" "sreagent" {
  name = "sreagent-read-only"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { AWS = data.sreagent_aws_external_id.this.trust_principal_arn }
      Condition = { StringEquals = { "sts:ExternalId" = data.sreagent_aws_external_id.this.external_id } }
    }]
  })
}

resource "sreagent_data_source" "cloudwatch" {
  name      = "cloudwatch"
  type      = "cloudwatch"
  regions   = ["us-east-1"]
  auth_type = "aws_assume_role"
  auth_credentials_wo = jsonencode({
    role_arn = aws_iam_role.sreagent.arn
  })
  auth_credentials_wo_version = 1
}
