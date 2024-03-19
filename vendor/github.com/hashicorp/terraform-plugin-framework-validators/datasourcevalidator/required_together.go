// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package datasourcevalidator

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/internal/configvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
)

// RequiredTogether checks that a set of path.Expression either has all known
// or all null values.
func RequiredTogether(expressions ...path.Expression) datasource.ConfigValidator {
	return &configvalidator.RequiredTogetherValidator{
		PathExpressions: expressions,
	}
}
