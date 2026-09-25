---
page_title: "Importing existing configuration"
subcategory: ""
description: |-
  Bring rows created in the app under Terraform with import blocks.
---

# Importing existing configuration

Every resource imports, either with `terraform import` or with an
[`import` block](https://developer.hashicorp.com/terraform/language/import) (Terraform 1.5 and later), which
`terraform plan -generate-config-out=generated.tf` can turn into configuration.

## Import IDs

| Resource | ID | Example |
| --- | --- | --- |
| Collections (alert routes, SLOs, teams and the rest) | the row's id | `00000000-0000-0000-0000-000000000000` |
| `sreagent_ticket_integration`, `sreagent_ticket_import_rule` | the provider | `jira` |
| Singletons (`sreagent_organization_settings` and the other settings) | the resource's own name | `organization_settings` |
| `sreagent_service_binding` | the service, plus `?environment=<name>` when the environment is not the default, each part escaped as in a URL | `checkout?environment=prod` |

A collection's id is on the row's page in the app and in its list data source:

```terraform
data "sreagent_alert_routes" "all" {}

output "alert_route_ids" {
  value = { for r in data.sreagent_alert_routes.all.items : r.service => r.id }
}
```

## Import blocks

```terraform
import {
  to = sreagent_alert_route.checkout
  id = "00000000-0000-0000-0000-000000000000"
}

import {
  to = sreagent_organization_settings.this
  id = "organization_settings"
}
```

## Resource identity

Every resource also carries a [resource identity](https://developer.hashicorp.com/terraform/language/import)
(Terraform 1.12 and later). A service binding is easiest to import by its two parts:

```terraform
import {
  to = sreagent_service_binding.checkout_prod
  identity = {
    service     = "checkout"
    environment = "prod"
  }
}
```

## Arguments the platform never answers

`sreagent_alert_mute`'s `duration_minutes` is sent when the mute is created and never read back (the platform
answers only `ends_at`). An imported mute therefore holds no `duration_minutes`, and a configuration that sets
it plans a replacement. Leave it out of an imported mute's configuration to keep the mute as it is.

## Secrets after an import

Nobody can read a secret back, so an imported resource holds no `<name>_wo_version`. Leave the write-only
argument out of the configuration to keep the stored secret, or add it with a version to replace it. See the
[secrets guide](secrets.md).
