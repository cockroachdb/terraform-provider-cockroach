package utils

import (
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ToStringMap takes a types.Map object and converts it to map[string]string.
func ToStringMap(m types.Map) (map[string]string, error) {
	labels := make(map[string]string)
	for k, v := range m.Elements() {
		valueStr, ok := v.(types.String)
		if !ok || valueStr.IsUnknown() || valueStr.IsNull() {
			return nil, errors.New("value in map is null, unknown, or not a string")
		}
		labels[k] = valueStr.ValueString()
	}
	return labels, nil
}
