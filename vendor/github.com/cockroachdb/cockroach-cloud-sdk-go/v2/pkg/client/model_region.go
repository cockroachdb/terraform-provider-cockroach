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

// Region struct for Region.
type Region struct {
	// internal_dns is the internal DNS name of the cluster within the cloud provider's network. It is used to connect to the cluster with AWS PrivateLink, Azure Private Link, and GCP VPC Peering, but not GCP Private Service Connect.
	InternalDns string `json:"internal_dns"`
	Name        string `json:"name"`
	// node_count will be 0 for Serverless clusters.
	NodeCount int32 `json:"node_count"`
	// primary is true only for the primary region in a Multi Region Serverless cluster.
	Primary *bool `json:"primary,omitempty"`
	// private_endpoint_dns is the DNS name of the cluster which is used to connect to the cluster with GCP Private Service Connect.
	PrivateEndpointDns string `json:"private_endpoint_dns"`
	// sql_dns is the DNS name of SQL interface of the cluster. It is used to connect to the cluster with IP allowlisting.
	SqlDns string `json:"sql_dns"`
	// ui_dns is the DNS name used when connecting to the DB Console for the cluster.
	UiDns string `json:"ui_dns"`
}

// NewRegion instantiates a new Region object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewRegion(internalDns string, name string, nodeCount int32, privateEndpointDns string, sqlDns string, uiDns string) *Region {
	p := Region{}
	p.InternalDns = internalDns
	p.Name = name
	p.NodeCount = nodeCount
	p.PrivateEndpointDns = privateEndpointDns
	p.SqlDns = sqlDns
	p.UiDns = uiDns
	return &p
}

// NewRegionWithDefaults instantiates a new Region object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewRegionWithDefaults() *Region {
	p := Region{}
	return &p
}

// GetInternalDns returns the InternalDns field value.
func (o *Region) GetInternalDns() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.InternalDns
}

// SetInternalDns sets field value.
func (o *Region) SetInternalDns(v string) {
	o.InternalDns = v
}

// GetName returns the Name field value.
func (o *Region) GetName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Name
}

// SetName sets field value.
func (o *Region) SetName(v string) {
	o.Name = v
}

// GetNodeCount returns the NodeCount field value.
func (o *Region) GetNodeCount() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.NodeCount
}

// SetNodeCount sets field value.
func (o *Region) SetNodeCount(v int32) {
	o.NodeCount = v
}

// GetPrimary returns the Primary field value if set, zero value otherwise.
func (o *Region) GetPrimary() bool {
	if o == nil || o.Primary == nil {
		var ret bool
		return ret
	}
	return *o.Primary
}

// SetPrimary gets a reference to the given bool and assigns it to the Primary field.
func (o *Region) SetPrimary(v bool) {
	o.Primary = &v
}

// GetPrivateEndpointDns returns the PrivateEndpointDns field value.
func (o *Region) GetPrivateEndpointDns() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.PrivateEndpointDns
}

// SetPrivateEndpointDns sets field value.
func (o *Region) SetPrivateEndpointDns(v string) {
	o.PrivateEndpointDns = v
}

// GetSqlDns returns the SqlDns field value.
func (o *Region) GetSqlDns() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.SqlDns
}

// SetSqlDns sets field value.
func (o *Region) SetSqlDns(v string) {
	o.SqlDns = v
}

// GetUiDns returns the UiDns field value.
func (o *Region) GetUiDns() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.UiDns
}

// SetUiDns sets field value.
func (o *Region) SetUiDns(v string) {
	o.UiDns = v
}