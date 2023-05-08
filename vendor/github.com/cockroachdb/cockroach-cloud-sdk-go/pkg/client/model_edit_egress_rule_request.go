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

// EditEgressRuleRequest EditEgressRuleRequest is the input message to the EditEgressRule RPC..
type EditEgressRuleRequest struct {
	// description is text that serves to document the rules purpose.
	Description *string `json:"description,omitempty"`
	// destination is a CIDR range or fully-qualified domain name to which outgoing traffic should be allowed. This field is required.
	Destination *string `json:"destination,omitempty"`
	// idempotency_key uniquely identifies this request. If not set, it will be set by the server.
	IdempotencyKey *string `json:"idempotency_key,omitempty"`
	// paths are the allowed URL paths. If empty, all paths are allowed. Only valid if Type=\"FQDN\".
	Paths *[]string `json:"paths,omitempty"`
	// ports are the allowed ports for TCP protocol. If empty, all ports are allowed.
	Ports *[]int32 `json:"ports,omitempty"`
	// type is the destination type of this rule. Example values are FQDN or CIDR. This field is required.
	Type *string `json:"type,omitempty"`
}

// NewEditEgressRuleRequest instantiates a new EditEgressRuleRequest object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewEditEgressRuleRequest() *EditEgressRuleRequest {
	p := EditEgressRuleRequest{}
	return &p
}

// GetDescription returns the Description field value if set, zero value otherwise.
func (o *EditEgressRuleRequest) GetDescription() string {
	if o == nil || o.Description == nil {
		var ret string
		return ret
	}
	return *o.Description
}

// SetDescription gets a reference to the given string and assigns it to the Description field.
func (o *EditEgressRuleRequest) SetDescription(v string) {
	o.Description = &v
}

// GetDestination returns the Destination field value if set, zero value otherwise.
func (o *EditEgressRuleRequest) GetDestination() string {
	if o == nil || o.Destination == nil {
		var ret string
		return ret
	}
	return *o.Destination
}

// SetDestination gets a reference to the given string and assigns it to the Destination field.
func (o *EditEgressRuleRequest) SetDestination(v string) {
	o.Destination = &v
}

// GetIdempotencyKey returns the IdempotencyKey field value if set, zero value otherwise.
func (o *EditEgressRuleRequest) GetIdempotencyKey() string {
	if o == nil || o.IdempotencyKey == nil {
		var ret string
		return ret
	}
	return *o.IdempotencyKey
}

// SetIdempotencyKey gets a reference to the given string and assigns it to the IdempotencyKey field.
func (o *EditEgressRuleRequest) SetIdempotencyKey(v string) {
	o.IdempotencyKey = &v
}

// GetPaths returns the Paths field value if set, zero value otherwise.
func (o *EditEgressRuleRequest) GetPaths() []string {
	if o == nil || o.Paths == nil {
		var ret []string
		return ret
	}
	return *o.Paths
}

// SetPaths gets a reference to the given []string and assigns it to the Paths field.
func (o *EditEgressRuleRequest) SetPaths(v []string) {
	o.Paths = &v
}

// GetPorts returns the Ports field value if set, zero value otherwise.
func (o *EditEgressRuleRequest) GetPorts() []int32 {
	if o == nil || o.Ports == nil {
		var ret []int32
		return ret
	}
	return *o.Ports
}

// SetPorts gets a reference to the given []int32 and assigns it to the Ports field.
func (o *EditEgressRuleRequest) SetPorts(v []int32) {
	o.Ports = &v
}

// GetType returns the Type field value if set, zero value otherwise.
func (o *EditEgressRuleRequest) GetType() string {
	if o == nil || o.Type == nil {
		var ret string
		return ret
	}
	return *o.Type
}

// SetType gets a reference to the given string and assigns it to the Type field.
func (o *EditEgressRuleRequest) SetType(v string) {
	o.Type = &v
}
