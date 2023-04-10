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

// GetConnectionStringResponse struct for GetConnectionStringResponse.
type GetConnectionStringResponse struct {
	// connection_string contains the full connection string with parameters formatted inline.
	ConnectionString *string `json:"connection_string,omitempty"`
	// params contains a list of individual key parameters for generating nonstandard connection strings.
	Params *map[string]string `json:"params,omitempty"`
}

// NewGetConnectionStringResponse instantiates a new GetConnectionStringResponse object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewGetConnectionStringResponse() *GetConnectionStringResponse {
	p := GetConnectionStringResponse{}
	return &p
}

// GetConnectionString returns the ConnectionString field value if set, zero value otherwise.
func (o *GetConnectionStringResponse) GetConnectionString() string {
	if o == nil || o.ConnectionString == nil {
		var ret string
		return ret
	}
	return *o.ConnectionString
}

// SetConnectionString gets a reference to the given string and assigns it to the ConnectionString field.
func (o *GetConnectionStringResponse) SetConnectionString(v string) {
	o.ConnectionString = &v
}

// GetParams returns the Params field value if set, zero value otherwise.
func (o *GetConnectionStringResponse) GetParams() map[string]string {
	if o == nil || o.Params == nil {
		var ret map[string]string
		return ret
	}
	return *o.Params
}

// SetParams gets a reference to the given map[string]string and assigns it to the Params field.
func (o *GetConnectionStringResponse) SetParams(v map[string]string) {
	o.Params = &v
}

func (o GetConnectionStringResponse) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if o.ConnectionString != nil {
		toSerialize["connection_string"] = o.ConnectionString
	}
	if o.Params != nil {
		toSerialize["params"] = o.Params
	}
	return json.Marshal(toSerialize)
}
