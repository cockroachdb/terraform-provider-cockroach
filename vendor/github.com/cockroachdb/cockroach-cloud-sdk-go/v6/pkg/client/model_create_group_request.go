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
// API version: 2024-09-16

package client

// CreateGroupRequest struct for CreateGroupRequest.
type CreateGroupRequest struct {
	DisplayName string          `json:"displayName"`
	ExternalId  *string         `json:"externalId,omitempty"`
	Members     *[]ScimResource `json:"members,omitempty"`
	Schemas     []string        `json:"schemas"`
}

// NewCreateGroupRequest instantiates a new CreateGroupRequest object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewCreateGroupRequest(displayName string, schemas []string) *CreateGroupRequest {
	p := CreateGroupRequest{}
	p.DisplayName = displayName
	p.Schemas = schemas
	return &p
}

// NewCreateGroupRequestWithDefaults instantiates a new CreateGroupRequest object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewCreateGroupRequestWithDefaults() *CreateGroupRequest {
	p := CreateGroupRequest{}
	return &p
}

// GetDisplayName returns the DisplayName field value.
func (o *CreateGroupRequest) GetDisplayName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.DisplayName
}

// SetDisplayName sets field value.
func (o *CreateGroupRequest) SetDisplayName(v string) {
	o.DisplayName = v
}

// GetExternalId returns the ExternalId field value if set, zero value otherwise.
func (o *CreateGroupRequest) GetExternalId() string {
	if o == nil || o.ExternalId == nil {
		var ret string
		return ret
	}
	return *o.ExternalId
}

// SetExternalId gets a reference to the given string and assigns it to the ExternalId field.
func (o *CreateGroupRequest) SetExternalId(v string) {
	o.ExternalId = &v
}

// GetMembers returns the Members field value if set, zero value otherwise.
func (o *CreateGroupRequest) GetMembers() []ScimResource {
	if o == nil || o.Members == nil {
		var ret []ScimResource
		return ret
	}
	return *o.Members
}

// SetMembers gets a reference to the given []ScimResource and assigns it to the Members field.
func (o *CreateGroupRequest) SetMembers(v []ScimResource) {
	o.Members = &v
}

// GetSchemas returns the Schemas field value.
func (o *CreateGroupRequest) GetSchemas() []string {
	if o == nil {
		var ret []string
		return ret
	}

	return o.Schemas
}

// SetSchemas sets field value.
func (o *CreateGroupRequest) SetSchemas(v []string) {
	o.Schemas = v
}
