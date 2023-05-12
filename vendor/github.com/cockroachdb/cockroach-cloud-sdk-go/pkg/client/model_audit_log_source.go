// Copyright 2023 The Cockroach Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.
// CockroachDB Cloud API
// API version: 2023-04-10

package client

import (
	"encoding/json"
	"fmt"
)

// AuditLogSource the model 'AuditLogSource'.
type AuditLogSource string

// List of AuditLogSource.
const (
	AUDITLOGSOURCE_CC_API   AuditLogSource = "AUDIT_LOG_SOURCE_CC_API"
	AUDITLOGSOURCE_CLI      AuditLogSource = "AUDIT_LOG_SOURCE_CLI"
	AUDITLOGSOURCE_UI       AuditLogSource = "AUDIT_LOG_SOURCE_UI"
	AUDITLOGSOURCE_INTERNAL AuditLogSource = "AUDIT_LOG_SOURCE_INTERNAL"
)

// All allowed values of AuditLogSource enum.
var AllowedAuditLogSourceEnumValues = []AuditLogSource{
	"AUDIT_LOG_SOURCE_CC_API",
	"AUDIT_LOG_SOURCE_CLI",
	"AUDIT_LOG_SOURCE_UI",
	"AUDIT_LOG_SOURCE_INTERNAL",
}

func (v *AuditLogSource) UnmarshalJSON(src []byte) error {
	var value string
	err := json.Unmarshal(src, &value)
	if err != nil {
		return err
	}
	enumTypeValue := AuditLogSource(value)
	for _, existing := range AllowedAuditLogSourceEnumValues {
		if existing == enumTypeValue {
			*v = enumTypeValue
			return nil
		}
	}

	return fmt.Errorf("%+v is not a valid AuditLogSource", value)
}

// NewAuditLogSourceFromValue returns a pointer to a valid AuditLogSource
// for the value passed as argument, or an error if the value passed is not allowed by the enum.
func NewAuditLogSourceFromValue(v string) (*AuditLogSource, error) {
	ev := AuditLogSource(v)
	if ev.IsValid() {
		return &ev, nil
	} else {
		return nil, fmt.Errorf("invalid value '%v' for AuditLogSource: valid values are %v", v, AllowedAuditLogSourceEnumValues)
	}
}

// IsValid return true if the value is valid for the enum, false otherwise.
func (v AuditLogSource) IsValid() bool {
	for _, existing := range AllowedAuditLogSourceEnumValues {
		if existing == v {
			return true
		}
	}
	return false
}

// Ptr returns reference to AuditLogSource value.
func (v AuditLogSource) Ptr() *AuditLogSource {
	return &v
}
