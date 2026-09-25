terraform {
  required_version = ">= 1.12"
  required_providers {
    sreagent = {
      source = "segfaultpw/sreagent"
    }
  }
}

variable "sreagent_api_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

# An sre_ak_ key holding the api:admin scope applies changes. Pinning the
# organization makes a key minted for another organization fail before
# anything is written.
provider "sreagent" {
  api_key      = var.sreagent_api_key
  organization = "acme"
}
