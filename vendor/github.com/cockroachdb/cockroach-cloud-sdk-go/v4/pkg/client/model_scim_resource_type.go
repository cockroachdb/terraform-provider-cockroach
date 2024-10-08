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

// ScimResourceType struct for ScimResourceType.
type ScimResourceType struct {
	Description *string       `json:"description,omitempty"`
	Endpoint    *string       `json:"endpoint,omitempty"`
	Id          *string       `json:"id,omitempty"`
	Meta        *ScimMetadata `json:"meta,omitempty"`
	Name        *string       `json:"name,omitempty"`
	Schema      *string       `json:"schema,omitempty"`
	Schemas     *[]string     `json:"schemas,omitempty"`
}

// NewScimResourceType instantiates a new ScimResourceType object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewScimResourceType() *ScimResourceType {
	p := ScimResourceType{}
	return &p
}

// GetDescription returns the Description field value if set, zero value otherwise.
func (o *ScimResourceType) GetDescription() string {
	if o == nil || o.Description == nil {
		var ret string
		return ret
	}
	return *o.Description
}

// SetDescription gets a reference to the given string and assigns it to the Description field.
func (o *ScimResourceType) SetDescription(v string) {
	o.Description = &v
}

// GetEndpoint returns the Endpoint field value if set, zero value otherwise.
func (o *ScimResourceType) GetEndpoint() string {
	if o == nil || o.Endpoint == nil {
		var ret string
		return ret
	}
	return *o.Endpoint
}

// SetEndpoint gets a reference to the given string and assigns it to the Endpoint field.
func (o *ScimResourceType) SetEndpoint(v string) {
	o.Endpoint = &v
}

// GetId returns the Id field value if set, zero value otherwise.
func (o *ScimResourceType) GetId() string {
	if o == nil || o.Id == nil {
		var ret string
		return ret
	}
	return *o.Id
}

// SetId gets a reference to the given string and assigns it to the Id field.
func (o *ScimResourceType) SetId(v string) {
	o.Id = &v
}

// GetMeta returns the Meta field value if set, zero value otherwise.
func (o *ScimResourceType) GetMeta() ScimMetadata {
	if o == nil || o.Meta == nil {
		var ret ScimMetadata
		return ret
	}
	return *o.Meta
}

// SetMeta gets a reference to the given ScimMetadata and assigns it to the Meta field.
func (o *ScimResourceType) SetMeta(v ScimMetadata) {
	o.Meta = &v
}

// GetName returns the Name field value if set, zero value otherwise.
func (o *ScimResourceType) GetName() string {
	if o == nil || o.Name == nil {
		var ret string
		return ret
	}
	return *o.Name
}

// SetName gets a reference to the given string and assigns it to the Name field.
func (o *ScimResourceType) SetName(v string) {
	o.Name = &v
}

// GetSchema returns the Schema field value if set, zero value otherwise.
func (o *ScimResourceType) GetSchema() string {
	if o == nil || o.Schema == nil {
		var ret string
		return ret
	}
	return *o.Schema
}

// SetSchema gets a reference to the given string and assigns it to the Schema field.
func (o *ScimResourceType) SetSchema(v string) {
	o.Schema = &v
}

// GetSchemas returns the Schemas field value if set, zero value otherwise.
func (o *ScimResourceType) GetSchemas() []string {
	if o == nil || o.Schemas == nil {
		var ret []string
		return ret
	}
	return *o.Schemas
}

// SetSchemas gets a reference to the given []string and assigns it to the Schemas field.
func (o *ScimResourceType) SetSchemas(v []string) {
	o.Schemas = &v
}
