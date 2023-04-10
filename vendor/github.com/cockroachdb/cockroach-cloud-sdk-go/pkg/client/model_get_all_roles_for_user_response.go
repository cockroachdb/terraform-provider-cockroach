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
)

// GetAllRolesForUserResponse struct for GetAllRolesForUserResponse.
type GetAllRolesForUserResponse struct {
	Roles *[]BuiltInRole `json:"roles,omitempty"`
}

// NewGetAllRolesForUserResponse instantiates a new GetAllRolesForUserResponse object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewGetAllRolesForUserResponse() *GetAllRolesForUserResponse {
	p := GetAllRolesForUserResponse{}
	return &p
}

// GetRoles returns the Roles field value if set, zero value otherwise.
func (o *GetAllRolesForUserResponse) GetRoles() []BuiltInRole {
	if o == nil || o.Roles == nil {
		var ret []BuiltInRole
		return ret
	}
	return *o.Roles
}

// SetRoles gets a reference to the given []BuiltInRole and assigns it to the Roles field.
func (o *GetAllRolesForUserResponse) SetRoles(v []BuiltInRole) {
	o.Roles = &v
}

func (o GetAllRolesForUserResponse) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if o.Roles != nil {
		toSerialize["roles"] = o.Roles
	}
	return json.Marshal(toSerialize)
}
