# The key ids your verifier should accept, in the same apply that configures it.
# The data source never answers a key: reveal it on Settings, Request signing, and
# give it to the verifier as a sensitive variable.
data "sreagent_request_signing" "this" {}

locals {
  signing_kids = [
    for key in jsondecode(data.sreagent_request_signing.this.keys) : key.kid
    if contains(["signing", "pending", "verify_only"], key.state)
  ]
}

output "request_signing_kids" {
  value = local.signing_kids
}

output "request_signing_guide" {
  value = data.sreagent_request_signing.this.docs_url
}
