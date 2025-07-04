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

// PatchUserRequest struct for PatchUserRequest.
type PatchUserRequest struct {
	Operations []ScimOperations `json:"Operations"`
	// A list of one or more URIs identifying SCIM schemas that define the structure of the attributes in the request. The only supported schema at this time is \"urn:ietf:params:scim:api:messages:2.0:PatchOp\".
	Schemas []string `json:"schemas"`
}

// NewPatchUserRequest instantiates a new PatchUserRequest object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewPatchUserRequest(operations []ScimOperations, schemas []string) *PatchUserRequest {
	p := PatchUserRequest{}
	p.Operations = operations
	p.Schemas = schemas
	return &p
}

// NewPatchUserRequestWithDefaults instantiates a new PatchUserRequest object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewPatchUserRequestWithDefaults() *PatchUserRequest {
	p := PatchUserRequest{}
	return &p
}

// GetOperations returns the Operations field value.
func (o *PatchUserRequest) GetOperations() []ScimOperations {
	if o == nil {
		var ret []ScimOperations
		return ret
	}

	return o.Operations
}

// SetOperations sets field value.
func (o *PatchUserRequest) SetOperations(v []ScimOperations) {
	o.Operations = v
}

// GetSchemas returns the Schemas field value.
func (o *PatchUserRequest) GetSchemas() []string {
	if o == nil {
		var ret []string
		return ret
	}

	return o.Schemas
}

// SetSchemas sets field value.
func (o *PatchUserRequest) SetSchemas(v []string) {
	o.Schemas = v
}
