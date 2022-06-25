// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.
// CockroachDB Cloud API
// API version: 2022-03-31

package client

import (
	"encoding/json"
	"fmt"
)

// CMEKCustomerAction CMEKCustomerAction enumerates the actions a customer can take on a cluster that has been enabled for CMEK.
type CMEKCustomerAction string

// List of CMEKCustomerAction.
const (
	CMEKCUSTOMERACTION_UNKNOWN_ACTION CMEKCustomerAction = "UNKNOWN_ACTION"
	CMEKCUSTOMERACTION_REVOKE         CMEKCustomerAction = "REVOKE"
)

// All allowed values of CMEKCustomerAction enum.
var AllowedCMEKCustomerActionEnumValues = []CMEKCustomerAction{
	"UNKNOWN_ACTION",
	"REVOKE",
}

func (v *CMEKCustomerAction) UnmarshalJSON(src []byte) error {
	var value string
	err := json.Unmarshal(src, &value)
	if err != nil {
		return err
	}
	enumTypeValue := CMEKCustomerAction(value)
	for _, existing := range AllowedCMEKCustomerActionEnumValues {
		if existing == enumTypeValue {
			*v = enumTypeValue
			return nil
		}
	}

	return fmt.Errorf("%+v is not a valid CMEKCustomerAction", value)
}

// NewCMEKCustomerActionFromValue returns a pointer to a valid CMEKCustomerAction
// for the value passed as argument, or an error if the value passed is not allowed by the enum.
func NewCMEKCustomerActionFromValue(v string) (*CMEKCustomerAction, error) {
	ev := CMEKCustomerAction(v)
	if ev.IsValid() {
		return &ev, nil
	} else {
		return nil, fmt.Errorf("invalid value '%v' for CMEKCustomerAction: valid values are %v", v, AllowedCMEKCustomerActionEnumValues)
	}
}

// IsValid return true if the value is valid for the enum, false otherwise.
func (v CMEKCustomerAction) IsValid() bool {
	for _, existing := range AllowedCMEKCustomerActionEnumValues {
		if existing == v {
			return true
		}
	}
	return false
}

// Ptr returns reference to CMEKCustomerAction value.
func (v CMEKCustomerAction) Ptr() *CMEKCustomerAction {
	return &v
}