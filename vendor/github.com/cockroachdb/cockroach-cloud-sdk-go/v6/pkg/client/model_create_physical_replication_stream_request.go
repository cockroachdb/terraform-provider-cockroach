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

// CreatePhysicalReplicationStreamRequest struct for CreatePhysicalReplicationStreamRequest.
type CreatePhysicalReplicationStreamRequest struct {
	// primary_cluster_id is the ID of the cluster that is being replicated.
	PrimaryClusterId string `json:"primary_cluster_id"`
	// standby_cluster_id is the ID of the cluster that data is being replicated to.
	StandbyClusterId string `json:"standby_cluster_id"`
}

// NewCreatePhysicalReplicationStreamRequest instantiates a new CreatePhysicalReplicationStreamRequest object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewCreatePhysicalReplicationStreamRequest(primaryClusterId string, standbyClusterId string) *CreatePhysicalReplicationStreamRequest {
	p := CreatePhysicalReplicationStreamRequest{}
	p.PrimaryClusterId = primaryClusterId
	p.StandbyClusterId = standbyClusterId
	return &p
}

// NewCreatePhysicalReplicationStreamRequestWithDefaults instantiates a new CreatePhysicalReplicationStreamRequest object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewCreatePhysicalReplicationStreamRequestWithDefaults() *CreatePhysicalReplicationStreamRequest {
	p := CreatePhysicalReplicationStreamRequest{}
	return &p
}

// GetPrimaryClusterId returns the PrimaryClusterId field value.
func (o *CreatePhysicalReplicationStreamRequest) GetPrimaryClusterId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.PrimaryClusterId
}

// SetPrimaryClusterId sets field value.
func (o *CreatePhysicalReplicationStreamRequest) SetPrimaryClusterId(v string) {
	o.PrimaryClusterId = v
}

// GetStandbyClusterId returns the StandbyClusterId field value.
func (o *CreatePhysicalReplicationStreamRequest) GetStandbyClusterId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.StandbyClusterId
}

// SetStandbyClusterId sets field value.
func (o *CreatePhysicalReplicationStreamRequest) SetStandbyClusterId(v string) {
	o.StandbyClusterId = v
}
