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
// API version: 2022-09-20

package client

import (
	"encoding/json"
)

// UserRoleGrants struct for UserRoleGrants.
type UserRoleGrants struct {
	Roles  []BuiltInRole `json:"roles"`
	UserId string        `json:"user_id"`
}

// NewUserRoleGrants instantiates a new UserRoleGrants object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewUserRoleGrants(roles []BuiltInRole, userId string) *UserRoleGrants {
	p := UserRoleGrants{}
	p.Roles = roles
	p.UserId = userId
	return &p
}

// NewUserRoleGrantsWithDefaults instantiates a new UserRoleGrants object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewUserRoleGrantsWithDefaults() *UserRoleGrants {
	p := UserRoleGrants{}
	return &p
}

// GetRoles returns the Roles field value.
func (o *UserRoleGrants) GetRoles() []BuiltInRole {
	if o == nil {
		var ret []BuiltInRole
		return ret
	}

	return o.Roles
}

// SetRoles sets field value.
func (o *UserRoleGrants) SetRoles(v []BuiltInRole) {
	o.Roles = v
}

// GetUserId returns the UserId field value.
func (o *UserRoleGrants) GetUserId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.UserId
}

// SetUserId sets field value.
func (o *UserRoleGrants) SetUserId(v string) {
	o.UserId = v
}

func (o UserRoleGrants) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["roles"] = o.Roles
	}
	if true {
		toSerialize["user_id"] = o.UserId
	}
	return json.Marshal(toSerialize)
}
