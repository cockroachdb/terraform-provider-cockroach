//go:build tools

package tools

import (
	_ "github.com/golang/mock/mockgen/model"
	// Documentation generation
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
