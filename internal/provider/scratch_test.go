package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
)

// scratch serves one or more specs that no released resource uses, against a fake
// facade built from the same specs, so an engine capability is proved end to end
// (the real Terraform CLI, the framework's plan consistency rules) before a real
// spec relies on it.
func scratch(t *testing.T, specs ...engine.Spec) (*fakefacade.Fake, map[string]func() (tfprotov6.ProviderServer, error)) {
	t.Helper()
	f := fakefacade.New(t, specs)
	return f, map[string]func() (tfprotov6.ProviderServer, error){
		"sreagent": providerserver.NewProtocol6WithError(newWithSpecs("test", specs)()),
	}
}
