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
// API version: development

package client

import (
	"fmt"
)

// CloudProviderType  - GCP: The Google Cloud Platform cloud provider.  - AWS: The Amazon Web Services cloud provider.  - AZURE: Limited Access: The Azure cloud provider.
type CloudProviderType string

// List of CloudProvider.Type.
const (
	CLOUDPROVIDERTYPE_GCP   CloudProviderType = "GCP"
	CLOUDPROVIDERTYPE_AWS   CloudProviderType = "AWS"
	CLOUDPROVIDERTYPE_AZURE CloudProviderType = "AZURE"
)

// All allowed values of CloudProviderType enum.
var AllowedCloudProviderTypeEnumValues = []CloudProviderType{
	"GCP",
	"AWS",
	"AZURE",
}

// NewCloudProviderTypeFromValue returns a pointer to a valid CloudProviderType
// for the value passed as argument, or an error if the value passed is not allowed by the enum.
func NewCloudProviderTypeFromValue(v string) (*CloudProviderType, error) {
	ev := CloudProviderType(v)
	if ev.IsValid() {
		return &ev, nil
	} else {
		return nil, fmt.Errorf("invalid value '%v' for CloudProviderType: valid values are %v", v, AllowedCloudProviderTypeEnumValues)
	}
}

// IsValid return true if the value is valid for the enum, false otherwise.
func (v CloudProviderType) IsValid() bool {
	for _, existing := range AllowedCloudProviderTypeEnumValues {
		if existing == v {
			return true
		}
	}
	return false
}

// Ptr returns reference to CloudProvider.Type value.
func (v CloudProviderType) Ptr() *CloudProviderType {
	return &v
}
