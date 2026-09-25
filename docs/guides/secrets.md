---
page_title: "Secrets and write-only arguments"
subcategory: ""
description: |-
  How the provider sends API keys, tokens and credentials without ever storing them in state or plan.
---

# Secrets and write-only arguments

No secret this provider sends is ever stored in Terraform state or in a plan. Every secret is a
[write-only argument](https://developer.hashicorp.com/terraform/language/manage-sensitive-data/write-only)
(Terraform 1.11 and later) named `<name>_wo`, paired with two more attributes:

- `<name>_wo_version`: a number you choose. The secret is sent when the resource is created and again whenever
  this number changes. Set it whenever you set `<name>_wo`; the provider refuses a write-only value without one.
- `<name>_set`: computed. Whether the platform holds a value for this secret.

Pair the write-only argument with an [ephemeral variable](https://developer.hashicorp.com/terraform/language/manage-sensitive-data/ephemeral)
(Terraform 1.10 and later), so the value never reaches state either way:

```terraform
variable "pagerduty_routing_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

# The routing key is write-only: it is sent when routing_key_wo_version
# changes and is never stored in state. Bump the version to rotate it.
resource "sreagent_outbound_config" "pagerduty" {
  name                   = "pagerduty"
  provider_type          = "pagerduty"
  routing_key_wo         = var.pagerduty_routing_key
  routing_key_wo_version = 1
}
```

## Rotating a secret

Change the secret's value where it comes from, then bump its version:

```terraform
routing_key_wo         = var.pagerduty_routing_key
routing_key_wo_version = 2
```

The next apply sends the new value once. Leaving the version alone never resends it, so an unchanged
configuration plans no change.

## What drift can be detected

The platform answers only whether a secret is stored, never its value. So:

- A secret removed in the app shows as `<name>_set = false` in the next refresh.
- A secret changed in the app to a different value cannot be detected. Bump `<name>_wo_version` to send the
  value your configuration holds.

## Credentials held as a whole

`sreagent_connector`'s `config_wo` and `sreagent_data_source`'s `auth_credentials_wo` are JSON objects stored
encrypted as a whole. Write them with `jsonencode(...)`. Changing `auth_type` on a data source to a type that
takes credentials needs `auth_credentials_wo` with a new version in the same apply.

## Imported resources

An imported resource has no `<name>_wo_version` in state, because nobody can read the secret back. Add the
write-only argument and a version to its configuration only when you want Terraform to set the secret.
