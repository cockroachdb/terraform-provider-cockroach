package validators

import (
	"context"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/helpers/validatordiag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var _ validator.String = parentIDValidator{}

type parentIDValidator struct{}

func (validator parentIDValidator) Description(_ context.Context) string {
	return `value must be a UUID or the string "root"`
}

func (validator parentIDValidator) MarkdownDescription(ctx context.Context) string {
	return validator.Description(ctx)
}

func (validator parentIDValidator) ValidateString(
	ctx context.Context, request validator.StringRequest, response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()

	if value == "root" {
		return
	}

	if _, err := uuid.Parse(value); err != nil {
		response.Diagnostics.Append(validatordiag.InvalidAttributeValueMatchDiagnostic(
			request.Path,
			validator.Description(ctx),
			value,
		))
	}
}

// FolderParentID returns an AttributeValidator which ensures that the value
// is formatted either as a UUID or matches the string "root".
func FolderParentID() validator.String {
	return parentIDValidator{}
}
