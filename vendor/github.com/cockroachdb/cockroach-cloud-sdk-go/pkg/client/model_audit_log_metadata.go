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

// AuditLogMetadata struct for AuditLogMetadata.
type AuditLogMetadata struct {
	IpAddress *string `json:"ip_address,omitempty"`
}

// NewAuditLogMetadata instantiates a new AuditLogMetadata object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewAuditLogMetadata() *AuditLogMetadata {
	p := AuditLogMetadata{}
	return &p
}

// GetIpAddress returns the IpAddress field value if set, zero value otherwise.
func (o *AuditLogMetadata) GetIpAddress() string {
	if o == nil || o.IpAddress == nil {
		var ret string
		return ret
	}
	return *o.IpAddress
}

// SetIpAddress gets a reference to the given string and assigns it to the IpAddress field.
func (o *AuditLogMetadata) SetIpAddress(v string) {
	o.IpAddress = &v
}
