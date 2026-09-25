# A CI job that only runs terraform plan (pull request checks, drift
# reports) holds a separate key with the api:config_read scope. It reads every
# resource and data source, so plan works; any create, update or delete is
# refused, so apply fails with: "The configured API key holds
# api:config_read, which can plan but not apply."
#
# Set SREAGENT_API_KEY to the api:config_read key in the CI job's
# environment rather than naming it here.
provider "sreagent" {
  organization = "acme"
}
