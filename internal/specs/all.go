// Package specs declares every configuration API resource the provider serves.
package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// All is every spec, in the API's own declaration order.
func All() []engine.Spec {
	return []engine.Spec{
		DeployPolicy,
	}
}
