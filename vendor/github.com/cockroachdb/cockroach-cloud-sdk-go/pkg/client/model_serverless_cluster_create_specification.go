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
// API version: development

package client

// ServerlessClusterCreateSpecification struct for ServerlessClusterCreateSpecification.
type ServerlessClusterCreateSpecification struct {
	// Preview: Specify which region should be made the primary region. This is only applicable to multi-region Serverless clusters. This field is required if you create the cluster in more than one region.
	PrimaryRegion *string `json:"primary_region,omitempty"`
	// Region values should match the cloud provider's zone code. For example, for Oregon, set region_name to \"us-west2\" for GCP and \"us-west-2\" for AWS.
	Regions     []string     `json:"regions"`
	UsageLimits *UsageLimits `json:"usage_limits,omitempty"`
	// WithEmptyIPAllowlist creates a cluster with no entries in the IP Allowlist. By default, Serverless clusters have one IP allowlist entry that opens the cluster to any IP. Set this property to true to ensure the cluster is closed to public traffic by default.
	WithEmptyIpAllowlist *bool `json:"with_empty_ip_allowlist,omitempty"`
}

// NewServerlessClusterCreateSpecification instantiates a new ServerlessClusterCreateSpecification object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewServerlessClusterCreateSpecification(regions []string) *ServerlessClusterCreateSpecification {
	p := ServerlessClusterCreateSpecification{}
	p.Regions = regions
	return &p
}

// NewServerlessClusterCreateSpecificationWithDefaults instantiates a new ServerlessClusterCreateSpecification object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewServerlessClusterCreateSpecificationWithDefaults() *ServerlessClusterCreateSpecification {
	p := ServerlessClusterCreateSpecification{}
	return &p
}

// GetPrimaryRegion returns the PrimaryRegion field value if set, zero value otherwise.
func (o *ServerlessClusterCreateSpecification) GetPrimaryRegion() string {
	if o == nil || o.PrimaryRegion == nil {
		var ret string
		return ret
	}
	return *o.PrimaryRegion
}

// SetPrimaryRegion gets a reference to the given string and assigns it to the PrimaryRegion field.
func (o *ServerlessClusterCreateSpecification) SetPrimaryRegion(v string) {
	o.PrimaryRegion = &v
}

// GetRegions returns the Regions field value.
func (o *ServerlessClusterCreateSpecification) GetRegions() []string {
	if o == nil {
		var ret []string
		return ret
	}

	return o.Regions
}

// SetRegions sets field value.
func (o *ServerlessClusterCreateSpecification) SetRegions(v []string) {
	o.Regions = v
}

// GetUsageLimits returns the UsageLimits field value if set, zero value otherwise.
func (o *ServerlessClusterCreateSpecification) GetUsageLimits() UsageLimits {
	if o == nil || o.UsageLimits == nil {
		var ret UsageLimits
		return ret
	}
	return *o.UsageLimits
}

// SetUsageLimits gets a reference to the given UsageLimits and assigns it to the UsageLimits field.
func (o *ServerlessClusterCreateSpecification) SetUsageLimits(v UsageLimits) {
	o.UsageLimits = &v
}

// GetWithEmptyIpAllowlist returns the WithEmptyIpAllowlist field value if set, zero value otherwise.
func (o *ServerlessClusterCreateSpecification) GetWithEmptyIpAllowlist() bool {
	if o == nil || o.WithEmptyIpAllowlist == nil {
		var ret bool
		return ret
	}
	return *o.WithEmptyIpAllowlist
}

// SetWithEmptyIpAllowlist gets a reference to the given bool and assigns it to the WithEmptyIpAllowlist field.
func (o *ServerlessClusterCreateSpecification) SetWithEmptyIpAllowlist(v bool) {
	o.WithEmptyIpAllowlist = &v
}
