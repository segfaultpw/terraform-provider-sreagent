package engine

import (
	"errors"
	"strings"
	"testing"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

func TestAPlanRefusalNamesTheFeatureTheSentenceAndTheExits(t *testing.T) {
	gated := &facadeResource{spec: Spec{Key: "security_sla_policy", TypeName: "security_sla_policy", PlanFeature: "the infrastructure suite"}}
	plain := &facadeResource{spec: Spec{Key: "alert_mutes", TypeName: "alert_mute"}}
	refused := &client.APIError{Status: 403, Code: "forbidden", Message: "The infrastructure suite is not included in your plan."}

	summary, detail := gated.failure("read", refused)
	if summary != "Could not read sreagent_security_sla_policy" {
		t.Errorf("summary = %q", summary)
	}
	for _, want := range []string{
		`The platform answered 403: "The infrastructure suite is not included in your plan."`,
		"sreagent_security_sla_policy needs the infrastructure suite in the organization's plan",
		"advanced integrations",
		"Nothing was changed.",
		"terraform state rm sreagent_security_sla_policy.<name>",
		"removed block whose lifecycle sets destroy = false",
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("the detail must say %q, got:\n%s", want, detail)
		}
	}
	if strings.Contains(detail, "outside Terraform") {
		t.Errorf("a refused plan is not a row deleted outside Terraform: %s", detail)
	}

	// A resource that names no plan feature keeps the plain error.
	if _, detail := plain.failure("read", refused); detail != refused.Error() {
		t.Errorf("a resource with no PlanFeature must keep the platform's error, got %q", detail)
	}

	// Another 403 keeps its own words.
	for name, err := range map[string]error{
		"a personal token":   &client.APIError{Status: 403, Code: "forbidden", Message: "The configuration API is reachable only with an API key holding the api:admin scope, never a personal session token."},
		"a missing scope":    &client.APIError{Status: 403, Code: "insufficient_scope", Message: "API key does not have the required scope: api:admin or api:config_read"},
		"a read-only key":    &client.APIError{Status: 403, Code: "read_only_key", Message: "This API key holds api:config_read, which reads configuration and never changes it."},
		"a network failure":  errors.New("connection refused"),
		"a missing resource": &client.APIError{Status: 404, Code: "not_found", Message: "No such configuration resource."},
	} {
		if _, detail := gated.failure("read", err); detail != err.Error() {
			t.Errorf("%s: detail = %q, want the error unchanged", name, detail)
		}
	}

	// The plan without the configuration API gets its own text.
	_, detail = gated.failure("create", &client.APIError{Status: 403, Code: "feature_not_available", Message: "The configuration API is not included in your plan."})
	for _, want := range []string{`The configuration API is not included in your plan.`, "advanced integrations", "terraform state rm sreagent_security_sla_policy.<name>"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the detail must say %q, got:\n%s", want, detail)
		}
	}
}
