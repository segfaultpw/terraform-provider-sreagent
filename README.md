# terraform-provider-sreagent
Terraform and OpenTofu provider for SRE Agent (sreagent.app): manage alert routes, outbound paging, synthetic checks, SLOs and the rest of an organization's configuration as code.

```terraform
terraform {
  required_providers {
    sreagent = {
      source = "segfaultpw/sreagent"
    }
  }
}

provider "sreagent" {
  organization = "acme" # the API key comes from SREAGENT_API_KEY
}

resource "sreagent_alert_route" "checkout" {
  service = "checkout"
  target  = "#checkout-alerts"
}
```

The provider is not published to a registry yet. The documentation lives in [docs/](docs/index.md), generated from
[templates/](templates) and [examples/](examples) by `go generate ./...`.

## How it works

The provider talks to the platform's configuration API (`/api/v1/config`), the same door the app's MCP tools use.
One generic resource and data source (`internal/engine`) read a declarative spec per API resource
(`internal/specs`). `internal/contract` holds every spec to the API's OpenAPI document, vendored from
sre-agent's committed snapshot; a daily job fails when production's document differs from it.

## Development

Needs Go (the version in `go.mod`) and Terraform 1.12 or later on `PATH`.

```shell
gofmt -l .                 # prints nothing
golangci-lint run ./...    # 0 issues
go test ./...              # unit tests, against an in-memory fake of the API
go generate ./...          # regenerates docs/
terraform fmt -check -recursive examples
```

Acceptance tests run against a throwaway local stack, never production:

```shell
./acceptance/seed.sh > /tmp/tf-acc.env   # starts Postgres and the pinned sre-agent image, seeds an organization
set -a; . /tmp/tf-acc.env; set +a
TF_ACC=1 go test ./internal/provider/ -run TestAcc -timeout 60m
docker compose -f acceptance/docker-compose.yml down -v
```
