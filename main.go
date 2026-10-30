// terraform-provider-nuclock manages a nuclock organisation as code.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/provider"
)

//go:generate go tool tfplugindocs generate --provider-name nuclock

// version is set by GoReleaser at release time.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers such as delve")
	flag.Parse()
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/corneizer/nuclock",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
