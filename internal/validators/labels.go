package validators

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/helpers/validatordiag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var labelKeyRegex = regexp.MustCompile("^[a-z][a-z0-9_-]*$")
var labelValueRegex = regexp.MustCompile("^[a-z0-9_-]*$")

const ResourceLabelLimit = 50

func isLabelKeyValid(s string) bool {
	// Requirements:
	// Starts with lowercase letter.
	// Contains only lowercase letters, numbers, "-", and "_".
	// Length between 1 and 63 characters.
	keyLength := len(s)
	return keyLength >= 1 && keyLength <= 63 && labelKeyRegex.MatchString(s)
}

func isLabelValueValid(s string) bool {
	// Requirements:
	// Contains only lowercase letters, numbers, "-", and "_".
	// Length between 0 and 63 characters.
	valueLength := len(s)
	return valueLength <= 63 && labelValueRegex.MatchString(s)
}

// getInvalidLabels returns a list of invalid labels found in the given label map.
func getInvalidLabels(labels map[string]string) []string {
	var invalidLabels []string
	for k, v := range labels {
		if !isLabelKeyValid(k) || !isLabelValueValid(v) {
			invalidLabel := fmt.Sprintf("%s:%s", k, v)
			invalidLabels = append(invalidLabels, invalidLabel)
		}
	}
	return invalidLabels
}

// isValidLabels ensures all labels in the given label map are formatted correctly.
func isValidLabels(labels map[string]string) bool {
	invalidLabels := getInvalidLabels(labels)
	return len(invalidLabels) <= 0
}

var _ validator.Map = labelsValidator{}

type labelsValidator struct{}

func (validator labelsValidator) Description(_ context.Context) string {
	return fmt.Sprintf("Labels must contain at most %d key-value pairs. Each label consists of a key that starts with a lowercase letter and an optional value. Both keys and values must be no more than 63 characters long and may only contain lowercase letters, numbers, hyphens (-), or underscores (_).", ResourceLabelLimit)
}

func (validator labelsValidator) MarkdownDescription(ctx context.Context) string {
	return validator.Description(ctx)
}

func (validator labelsValidator) ValidateMap(
	_ context.Context, request validator.MapRequest, response *validator.MapResponse,
) {
	value := request.ConfigValue
	if value.IsNull() || value.IsUnknown() || len(value.Elements()) == 0 {
		return
	}

	// A label value is unknown during the validate walk when it references a value
	// Terraform has not resolved yet, as in `labels = { environment = var.environment }`.
	// Validators must tolerate unknown values and defer to the walk that runs once
	// they resolve, rather than failing the whole plan.
	labels := make(map[string]string, len(value.Elements()))
	var nullKeys []string
	for key, elem := range value.Elements() {
		strVal, ok := elem.(types.String)
		if !ok {
			// Only reachable if this validator is attached to a map whose element
			// type is not a string, which the schema is meant to rule out.
			response.Diagnostics.Append(validatordiag.BugInProviderDiagnostic(
				fmt.Sprintf("Labels validator received a non-string value for key %q", key),
			))
			return
		}
		switch {
		case strVal.IsNull():
			nullKeys = append(nullKeys, key)
			labels[key] = ""
		case strVal.IsUnknown():
			labels[key] = ""
		default:
			labels[key] = strVal.ValueString()
		}
	}

	if len(nullKeys) > 0 {
		sort.Strings(nullKeys)
		response.Diagnostics.Append(validatordiag.InvalidAttributeValueDiagnostic(
			request.Path,
			fmt.Sprintf("must not contain null values, but found null for: %s", strings.Join(nullKeys, ", ")),
			value.String(),
		))
	}

	if len(labels) > ResourceLabelLimit {
		response.Diagnostics.Append(validatordiag.InvalidAttributeValueDiagnostic(
			request.Path,
			fmt.Sprintf("must contain at most %d key-value pairs", ResourceLabelLimit),
			value.String(),
		))
	}

	if !isValidLabels(labels) {
		response.Diagnostics.Append(validatordiag.InvalidAttributeValueDiagnostic(
			request.Path,
			"consist of pairs of keys and optional values. Keys must start with a lowercase letter. Both keys and values must be no more than 63 characters long and may only contain lowercase letters, numbers, hyphens (-), or underscores (_)",
			value.String(),
		))
	}
}

// Labels returns an AttributeValidator which ensures that the
// labels passed in are formatted correctly.
func Labels() validator.Map {
	return labelsValidator{}
}
