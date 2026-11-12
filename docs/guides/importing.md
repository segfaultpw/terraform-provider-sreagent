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

## Repository settings that follow the organization default

A `sreagent_repo_setting` holds only the settings it sets itself; the rest follow the organization's repository
defaults. `set_here` lists the settings a row holds and `following` the ones it does not.

- An imported row that follows a setting plans nothing for it: leave the setting out of the configuration, or
  name it in `follows_default` (the exported file does). `follows_default` is never stored, so an import has
  nothing to plan for it, and it changes something only for a setting the row holds.
- A followed `draft_prs` reads null, so a configuration that leaves it out is already satisfied. Read
  `data.sreagent_effective_repo_settings` for the value a repository actually runs with.
- A setting the configuration states with the value the row already reads is not written, so restating what a
  row follows does not pin it. To make a row hold a value, state a different one.
- `fix_runner`, `platform_fallback` and `auto_dispatch` belong to the whole repository: a write or a reset of
  one reaches every row of the repository. State them the same way on every block of a repository, or on the
  all-branches block only.

```terraform
resource "sreagent_repo_setting" "web" {
  repo            = "acme/web"
  fix_runner      = "self_hosted"
  follows_default = ["platform_fallback", "auto_dispatch", "draft_prs", "check_network", "allow_ci_config"]
}
```

## The organization's repository defaults

`sreagent_repository_defaults` imports with the ID `repository_defaults`. A child organization that follows its
parent's defaults has no row of its own, so there is nothing to import: the import fails and says why. Declaring
the resource instead gives the child its own defaults for all six settings and stops it following the parent (the
plan says so and lists what moves), and destroying the resource follows the parent again.

## Arguments the platform never answers

`sreagent_alert_mute`'s `duration_minutes` is sent when the mute is created and never read back (the platform
answers only `ends_at`). An imported mute therefore holds no `duration_minutes`, and a configuration that sets
it plans a replacement. Leave it out of an imported mute's configuration to keep the mute as it is.

## Secrets after an import

Nobody can read a secret back, so an imported resource holds no `<name>_wo_version`. Leave the write-only
argument out of the configuration to keep the stored secret, or add it with a version to replace it. See the
[secrets guide](secrets.md).
