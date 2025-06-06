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

// ScimOperations struct for ScimOperations.
type ScimOperations struct {
	Op    string       `json:"op"`
	Path  *string      `json:"path,omitempty"`
	Value *interface{} `json:"value,omitempty"`
}

// NewScimOperations instantiates a new ScimOperations object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewScimOperations(op string) *ScimOperations {
	p := ScimOperations{}
	p.Op = op
	return &p
}

// NewScimOperationsWithDefaults instantiates a new ScimOperations object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewScimOperationsWithDefaults() *ScimOperations {
	p := ScimOperations{}
	return &p
}

// GetOp returns the Op field value.
func (o *ScimOperations) GetOp() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Op
}

// SetOp sets field value.
func (o *ScimOperations) SetOp(v string) {
	o.Op = v
}

// GetPath returns the Path field value if set, zero value otherwise.
func (o *ScimOperations) GetPath() string {
	if o == nil || o.Path == nil {
		var ret string
		return ret
	}
	return *o.Path
}

// SetPath gets a reference to the given string and assigns it to the Path field.
func (o *ScimOperations) SetPath(v string) {
	o.Path = &v
}

// GetValue returns the Value field value if set, zero value otherwise.
func (o *ScimOperations) GetValue() interface{} {
	if o == nil || o.Value == nil {
		var ret interface{}
		return ret
	}
	return *o.Value
}

// SetValue gets a reference to the given interface{} and assigns it to the Value field.
func (o *ScimOperations) SetValue(v interface{}) {
	o.Value = &v
}
