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

// ScimResource struct for ScimResource.
type ScimResource struct {
	Display *string `json:"display,omitempty"`
	Ref     *string `json:"ref,omitempty"`
	Type    *string `json:"type,omitempty"`
	Value   *string `json:"value,omitempty"`
}

// NewScimResource instantiates a new ScimResource object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewScimResource() *ScimResource {
	p := ScimResource{}
	return &p
}

// GetDisplay returns the Display field value if set, zero value otherwise.
func (o *ScimResource) GetDisplay() string {
	if o == nil || o.Display == nil {
		var ret string
		return ret
	}
	return *o.Display
}

// SetDisplay gets a reference to the given string and assigns it to the Display field.
func (o *ScimResource) SetDisplay(v string) {
	o.Display = &v
}

// GetRef returns the Ref field value if set, zero value otherwise.
func (o *ScimResource) GetRef() string {
	if o == nil || o.Ref == nil {
		var ret string
		return ret
	}
	return *o.Ref
}

// SetRef gets a reference to the given string and assigns it to the Ref field.
func (o *ScimResource) SetRef(v string) {
	o.Ref = &v
}

// GetType returns the Type field value if set, zero value otherwise.
func (o *ScimResource) GetType() string {
	if o == nil || o.Type == nil {
		var ret string
		return ret
	}
	return *o.Type
}

// SetType gets a reference to the given string and assigns it to the Type field.
func (o *ScimResource) SetType(v string) {
	o.Type = &v
}

// GetValue returns the Value field value if set, zero value otherwise.
func (o *ScimResource) GetValue() string {
	if o == nil || o.Value == nil {
		var ret string
		return ret
	}
	return *o.Value
}

// SetValue gets a reference to the given string and assigns it to the Value field.
func (o *ScimResource) SetValue(v string) {
	o.Value = &v
}