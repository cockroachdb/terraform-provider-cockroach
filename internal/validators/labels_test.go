package validators

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

// TestLabelsValidator exercises the labels validator directly. The resource
// level tests build their configuration with quoted string literals, so every
// label value they produce is known. Unknown values, which is what Terraform
// supplies for `labels = { environment = var.environment }` during the validate
// walk, can only be expressed by constructing the attribute values by hand.
func TestLabelsValidator(t *testing.T) {
	tooManyLabels := make(map[string]attr.Value, ResourceLabelLimit+1)
	tooManyUnknownLabels := make(map[string]attr.Value, ResourceLabelLimit+1)
	for i := 0; i <= ResourceLabelLimit; i++ {
		tooManyLabels[fmt.Sprintf("key%d", i)] = types.StringValue("value")
		tooManyUnknownLabels[fmt.Sprintf("key%d", i)] = types.StringUnknown()
	}

	// The format and limit diagnostics are shared by several cases, so asserting
	// on their text is what distinguishes "rejected for the right reason" from
	// "rejected because an unknown value leaked into a check".
	formatErr := "consist of pairs of keys and optional values"
	limitErr := fmt.Sprintf("must contain at most %d key-value pairs", ResourceLabelLimit)
	nullErr := "must not contain null values, but found null for: environment"

	testCases := []struct {
		name                string
		labels              types.Map
		expectedErrContains string
	}{
		{
			name:   "null map",
			labels: types.MapNull(types.StringType),
		},
		{
			name:   "unknown map",
			labels: types.MapUnknown(types.StringType),
		},
		{
			name:   "empty map",
			labels: types.MapValueMust(types.StringType, map[string]attr.Value{}),
		},
		{
			name: "known values",
			labels: types.MapValueMust(types.StringType, map[string]attr.Value{
				"environment": types.StringValue("dev"),
				"team":        types.StringValue("console"),
			}),
		},
		{
			name: "unknown value",
			labels: types.MapValueMust(types.StringType, map[string]attr.Value{
				"environment": types.StringUnknown(),
			}),
		},
		{
			name: "null value",
			labels: types.MapValueMust(types.StringType, map[string]attr.Value{
				"environment": types.StringNull(),
			}),
			expectedErrContains: nullErr,
		},
		{
			// Only the null key is reported. The unknown one is deferred.
			name: "null value alongside unknown value",
			labels: types.MapValueMust(types.StringType, map[string]attr.Value{
				"environment": types.StringNull(),
				"team":        types.StringUnknown(),
			}),
			expectedErrContains: nullErr,
		},
		{
			name: "invalid key",
			labels: types.MapValueMust(types.StringType, map[string]attr.Value{
				"Environment": types.StringValue("dev"),
			}),
			expectedErrContains: formatErr,
		},
		{
			// The key is still checked even though the value is deferred.
			name: "invalid key with unknown value",
			labels: types.MapValueMust(types.StringType, map[string]attr.Value{
				"Environment": types.StringUnknown(),
			}),
			expectedErrContains: formatErr,
		},
		{
			name: "invalid value",
			labels: types.MapValueMust(types.StringType, map[string]attr.Value{
				"environment": types.StringValue("NOT VALID!"),
			}),
			expectedErrContains: formatErr,
		},
		{
			// The known sibling is still checked on this pass.
			name: "invalid value alongside unknown value",
			labels: types.MapValueMust(types.StringType, map[string]attr.Value{
				"environment": types.StringUnknown(),
				"team":        types.StringValue("NOT VALID!"),
			}),
			expectedErrContains: formatErr,
		},
		{
			name:                "above label limit",
			labels:              types.MapValueMust(types.StringType, tooManyLabels),
			expectedErrContains: limitErr,
		},
		{
			// Unknown values still count toward the limit.
			name:                "above label limit with unknown values",
			labels:              types.MapValueMust(types.StringType, tooManyUnknownLabels),
			expectedErrContains: limitErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := validator.MapRequest{
				Path:        path.Root("labels"),
				ConfigValue: tc.labels,
			}
			response := &validator.MapResponse{}

			Labels().ValidateMap(context.Background(), request, response)

			if tc.expectedErrContains == "" {
				require.False(t, response.Diagnostics.HasError(),
					"unexpected errors: %v", response.Diagnostics.Errors())
			} else {
				require.Contains(t, errorDetails(response.Diagnostics), tc.expectedErrContains)
			}
		})
	}
}

// errorDetails joins the detail of every error diagnostic so a test can assert
// on why validation failed rather than only on whether it failed.
func errorDetails(diags diag.Diagnostics) string {
	details := make([]string, 0, len(diags.Errors()))
	for _, d := range diags.Errors() {
		details = append(details, d.Detail())
	}
	return strings.Join(details, "\n")
}
