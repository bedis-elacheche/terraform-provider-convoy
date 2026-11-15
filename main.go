// Command terraform-provider-convoy serves the Convoy Terraform provider.

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name convoy

package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider"
)

// version is set by goreleaser.
var (
	version = "dev"
	commit  = ""
)

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/bedis-elacheche/convoy",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
