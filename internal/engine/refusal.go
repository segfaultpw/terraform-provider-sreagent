package engine

import (
	"errors"
	"fmt"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

// failure is the summary and detail of an operation that failed with err. A
// resource that needs a plan feature (Spec.PlanFeature) turns the platform's
// plan refusal into a message that names the feature and the ways out: the
// platform answers 403 for a read too, and a refresh that fails this way must
// never read as a row deleted outside Terraform. A read-only key, a missing
// scope and a personal token keep their own messages.
func (r *facadeResource) failure(verb string, err error) (string, string) {
	if r.spec.PlanFeature != "" {
		var apiErr *client.APIError
		switch {
		case client.IsPlanRefusal(err) && errors.As(err, &apiErr):
			return r.title(verb), fmt.Sprintf("The platform answered 403: %q sreagent_%s needs %s in the organization's plan (and the configuration API needs a plan with advanced integrations). Nothing was changed. %s",
				apiErr.Message, r.spec.TypeName, r.spec.PlanFeature, r.planExits(verb))
		case client.IsConfigAPIUnavailable(err) && errors.As(err, &apiErr):
			return r.title(verb), fmt.Sprintf("The platform answered 403: %q sreagent_%s is managed through the configuration API, which needs a plan with advanced integrations. Nothing was changed. %s",
				apiErr.Message, r.spec.TypeName, r.planExits(verb))
		}
	}
	return r.title(verb), err.Error()
}

// planExits are the ways to stop managing a resource the plan no longer allows. A
// create has nothing in state to forget, so the block is the only thing to remove.
func (r *facadeResource) planExits(verb string) string {
	if verb == "create" {
		return "Until the plan includes it, remove the block."
	}
	return fmt.Sprintf("Until the plan includes it, stop managing it here with terraform state rm sreagent_%s.<name>, or with a removed block whose lifecycle sets destroy = false.", r.spec.TypeName)
}
