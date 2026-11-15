//go:build tools

// Package tools pins build-time tool dependencies.
package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
