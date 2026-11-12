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
- For `fix_runner`, `platform_fallback`, `auto_dispatch`, `check_network` and `allow_ci_config`, a setting the
  configuration states with the value the row already reads is not written, so restating what a row follows does
  not pin it. That holds while the default does not move: a stated value is kept, so after the default changes the
  next apply writes the stated value and the row holds it. Remove a stated value that should follow the default, or
  name it in `follows_default`. The preview of a change to `sreagent_repository_defaults` cannot see the blocks
  that would pin their rows back.
- `draft_prs` is the exception: leaving it out of a block whose row holds a value (an imported row, or one set in
  the app) plans a change to null, which puts the row back on the default, and stating it writes it even with the
  value the row follows. The exported file writes `draft_prs` only for a row that holds it.
- `fix_runner`, `platform_fallback` and `auto_dispatch` belong to the whole repository: a write or a reset of
  one reaches every row of the repository, and the plan warns. State them on the all-branches block only, and
  leave them out of the other blocks of the repository. Two blocks that both change one in a single apply need a
  second apply: the first write changes the version of the other row, and the second write is refused (412).

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
plan says so and lists what moves). Destroying the resource hands the child back to the parent and is planned like
a change: the destroy plan lists what moves, and if the parent's defaults turn CI configuration or network access
on, it fails until `confirm_on_destroy` holds the confirmation (set it, apply, then destroy).

## Arguments the platform never answers

`sreagent_alert_mute`'s `duration_minutes` is sent when the mute is created and never read back (the platform
answers only `ends_at`). An imported mute therefore holds no `duration_minutes`, and a configuration that sets
it plans a replacement. Leave it out of an imported mute's configuration to keep the mute as it is.

## Secrets after an import

Nobody can read a secret back, so an imported resource holds no `<name>_wo_version`. Leave the write-only
argument out of the configuration to keep the stored secret, or add it with a version to replace it. See the
[secrets guide](secrets.md).
