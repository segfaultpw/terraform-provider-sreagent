variable "anthropic_api_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "sreagent_ai_provider" "anthropic" {
  name               = "anthropic"
  provider_type      = "anthropic"
  model              = "claude-sonnet-5"
  api_key_wo         = var.anthropic_api_key
  api_key_wo_version = 1
}

# Bedrock takes no API key: SRE Agent assumes an IAM role in your own AWS
# account. The role trusts the platform principal and your organization's
# ExternalId, so create it in the same apply and hand its ARN over. Narrow the
# policy's Resource to the models you enable.
data "sreagent_aws_external_id" "this" {}

resource "aws_iam_role" "bedrock" {
  name = "sre-agent-bedrock"
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

resource "aws_iam_role_policy" "bedrock_invoke" {
  name = "invoke-models"
  role = aws_iam_role.bedrock.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["bedrock:InvokeModel", "bedrock:InvokeModelWithResponseStream", "bedrock:ListFoundationModels"]
      Resource = "*"
    }]
  })
}

resource "sreagent_ai_provider" "bedrock" {
  name          = "bedrock"
  provider_type = "bedrock"
  role_arn      = aws_iam_role.bedrock.arn
  region        = "us-east-1"
}
