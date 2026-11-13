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
| `sreagent_security_sla_policy`, `sreagent_security_digest_settings`, `sreagent_security_code_classes` | the resource's own name | `security_sla_policy` |
| `sreagent_security_notification_rule`, `sreagent_secret_allow_rule` | the row's id | `00000000-0000-0000-0000-000000000000` |
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

## The security settings and rules

The security SLA policy, the digest, the code classes, the notification rules and the secret allow rules are managed by
`sreagent_security_sla_policy`, `sreagent_security_digest_settings`, `sreagent_security_code_classes`,
`sreagent_security_notification_rule` and `sreagent_secret_allow_rule`. They need the infrastructure suite in the plan.
Their data sources are unchanged, and each id they answer is the id to import. Once the platform's export names provider
0.3.0 as its floor, the exported file writes all five; until then it does not, and a platform that has not been updated
keeps its older floor.

- **Two nulls are values.** An SLA with no days for a severity, and a digest with no Slack channel, are stored as null
  on the platform. The resources write them as `0` (days) and `""` (channel) and send null for them; the data sources
  keep reading null. An imported singleton that holds one plans nothing once the configuration says `info_days = 0` or
  `slack_channel = ""`, and a configuration that leaves the attribute out leaves it as it is.
- **A singleton attribute left out is left as it is.** This holds for the three settings, so importing one and stating
  only the attributes you care about changes nothing else. Destroying one leaves the settings in place, with a warning
  that says what stays in force. A block you do not import adopts the settings the organization has: the plan reads them
  and warns what the apply will change (classes unmarked, SLA days moved, digest fields), so import first to review the
  change as a plan.
- **A notification rule takes the platform's defaults for what it leaves out.** That includes `page` (false) and
  `events` (`new_finding`), so removing `page = true` stops paging. The rule the platform seeds, Critical and high
  findings, has four events (`new_finding`, `sla_breached`, `acceptance_expired`, `fix_verified`); state every field of it
  to import it without a change. Destroying a rule deletes it, the seeded one included, and the platform never creates it
  again.
- **The code classes are a set.** The order does not matter, the platform stores them sorted, and `[]` unmarks them all.
- **An allow rule cannot be edited.** Any change replaces it. The repository is stored in lower case and the end time
  with microseconds; neither is a change. Removing `repository`, `detector_type` or `expires_at` replaces the rule with a
  wider or longer lived one, and the plan warns what widens. A rule whose end has passed imports without a change, but
  editing any other field of it fails the plan, because the replacement would end in the past: set a later `expires_at`
  or remove the block. For a rule that has ended and is left as it is, the plan warns that it covers nothing.

```terraform
import {
  to = sreagent_security_sla_policy.this
  id = "security_sla_policy"
}

import {
  to = sreagent_secret_allow_rule.fixtures
  id = "00000000-0000-0000-0000-000000000000"
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
