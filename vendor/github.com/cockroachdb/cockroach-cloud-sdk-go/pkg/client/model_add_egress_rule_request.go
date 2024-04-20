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

// AddEgressRuleRequest AddEgressRuleRequest is the input for the rpc AddEgressRule()..
type AddEgressRuleRequest struct {
	// description is text that serves to document the rules purpose.
	Description string `json:"description"`
	// destination is the endpoint (or subnetwork if CIDR) to which traffic is allowed.
	Destination string `json:"destination"`
	// idempotency_key uniquely identifies this request. If not set, it will be set by the server.
	IdempotencyKey *string `json:"idempotency_key,omitempty"`
	// name is the name of the egress rule.
	Name string `json:"name"`
	// Deprecated: This field is ignored and will be removed in the next version. paths are the allowed URL paths. If empty, all paths are allowed. Only valid if Type=\"FQDN\".
	Paths *[]string `json:"paths,omitempty"`
	// ports are the allowed ports for TCP protocol. If Empty, all ports are allowed.
	Ports *[]int32 `json:"ports,omitempty"`
	// type classifies the Destination field. Valid types include: \"FQDN\", \"CIDR\".
	Type string `json:"type"`
}

// NewAddEgressRuleRequest instantiates a new AddEgressRuleRequest object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewAddEgressRuleRequest(description string, destination string, name string, type_ string) *AddEgressRuleRequest {
	p := AddEgressRuleRequest{}
	p.Description = description
	p.Destination = destination
	p.Name = name
	p.Type = type_
	return &p
}

// NewAddEgressRuleRequestWithDefaults instantiates a new AddEgressRuleRequest object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewAddEgressRuleRequestWithDefaults() *AddEgressRuleRequest {
	p := AddEgressRuleRequest{}
	return &p
}

// GetDescription returns the Description field value.
func (o *AddEgressRuleRequest) GetDescription() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Description
}

// SetDescription sets field value.
func (o *AddEgressRuleRequest) SetDescription(v string) {
	o.Description = v
}

// GetDestination returns the Destination field value.
func (o *AddEgressRuleRequest) GetDestination() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Destination
}

// SetDestination sets field value.
func (o *AddEgressRuleRequest) SetDestination(v string) {
	o.Destination = v
}

// GetIdempotencyKey returns the IdempotencyKey field value if set, zero value otherwise.
func (o *AddEgressRuleRequest) GetIdempotencyKey() string {
	if o == nil || o.IdempotencyKey == nil {
		var ret string
		return ret
	}
	return *o.IdempotencyKey
}

// SetIdempotencyKey gets a reference to the given string and assigns it to the IdempotencyKey field.
func (o *AddEgressRuleRequest) SetIdempotencyKey(v string) {
	o.IdempotencyKey = &v
}

// GetName returns the Name field value.
func (o *AddEgressRuleRequest) GetName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Name
}

// SetName sets field value.
func (o *AddEgressRuleRequest) SetName(v string) {
	o.Name = v
}

// GetPaths returns the Paths field value if set, zero value otherwise.
func (o *AddEgressRuleRequest) GetPaths() []string {
	if o == nil || o.Paths == nil {
		var ret []string
		return ret
	}
	return *o.Paths
}

// SetPaths gets a reference to the given []string and assigns it to the Paths field.
func (o *AddEgressRuleRequest) SetPaths(v []string) {
	o.Paths = &v
}

// GetPorts returns the Ports field value if set, zero value otherwise.
func (o *AddEgressRuleRequest) GetPorts() []int32 {
	if o == nil || o.Ports == nil {
		var ret []int32
		return ret
	}
	return *o.Ports
}

// SetPorts gets a reference to the given []int32 and assigns it to the Ports field.
func (o *AddEgressRuleRequest) SetPorts(v []int32) {
	o.Ports = &v
}

// GetType returns the Type field value.
func (o *AddEgressRuleRequest) GetType() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Type
}

// SetType sets field value.
func (o *AddEgressRuleRequest) SetType(v string) {
	o.Type = v
}
