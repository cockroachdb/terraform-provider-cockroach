package validators

import (
	"context"
	"fmt"
	"regexp"

	"github.com/cockroachdb/terraform-provider-cockroach/internal/utils"
	"github.com/hashicorp/terraform-plugin-framework-validators/helpers/validatordiag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	return fmt.Sprintf("must contain at most %d key-value pairs, with each key and each value allowed up to 63 characters (lowercase letters, numbers, hyphens, or underscores)", ResourceLabelLimit)
}

func (validator labelsValidator) MarkdownDescription(ctx context.Context) string {
	return validator.Description(ctx)
}

func (validator labelsValidator) ValidateMap(
	ctx context.Context, request validator.MapRequest, response *validator.MapResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() || len(request.ConfigValue.Elements()) == 0 {
		return
	}

	value, diags := request.ConfigValue.ToMapValue(ctx)
	response.Diagnostics.Append(diags...)

	labels, err := utils.ToStringMap(value)
	if err != nil {
		response.Diagnostics.AddError(
			"Error processing labels",
			fmt.Sprintf("Could not convert labels: %v", err),
		)
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
			"must have keys and values with a maximum of 63 characters, using only lowercase letters, numbers, hyphens (-), or underscores (_)",
			value.String(),
		))
	}
}

// Labels returns an AttributeValidator which ensures that the
// labels passed in are formatted correctly.
func Labels() validator.Map {
	return labelsValidator{}
}
