// Command terraform-provider-sreagent serves the sreagent Terraform provider.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/provider"
)

//go:generate go tool tfplugindocs generate --provider-name sreagent

// version is set by goreleaser at release time.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/segfaultpw/sreagent",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
