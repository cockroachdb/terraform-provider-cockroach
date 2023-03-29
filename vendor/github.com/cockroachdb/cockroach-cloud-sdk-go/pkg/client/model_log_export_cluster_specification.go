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

// LogExportClusterSpecification LogExportClusterSpecification contains all the data necessary to configure log export for an individual cluster. Users would supply this data via the API and also receive it back when inspecting the state of their log export configuration..
type LogExportClusterSpecification struct {
	// auth_principal is either the AWS Role ARN that identifies a role that the cluster account can assume to write to CloudWatch or the GCP Project ID that the cluster service account has permissions to write to for cloud logging.
	AuthPrincipal *string `json:"auth_principal,omitempty"`
	// groups is a collection of log group configurations to customize which CRDB channels get aggregated into different groups at the target sink. Unconfigured channels will be sent to the default locations via the settings above.
	Groups *[]LogExportGroup `json:"groups,omitempty"`
	// log_name is an identifier for the logs in the customer's log sink.
	LogName *string `json:"log_name,omitempty"`
	// redact controls whether logs are redacted before forwarding to customer sinks. By default they are not redacted.
	Redact *bool `json:"redact,omitempty"`
	// region controls whether all logs are sent to a specific region in the customer sink. By default, logs will remain their region of origin depending on the cluster node's region.
	Region *string        `json:"region,omitempty"`
	Type   *LogExportType `json:"type,omitempty"`
}

// NewLogExportClusterSpecification instantiates a new LogExportClusterSpecification object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewLogExportClusterSpecification() *LogExportClusterSpecification {
	p := LogExportClusterSpecification{}
	return &p
}

// GetAuthPrincipal returns the AuthPrincipal field value if set, zero value otherwise.
func (o *LogExportClusterSpecification) GetAuthPrincipal() string {
	if o == nil || o.AuthPrincipal == nil {
		var ret string
		return ret
	}
	return *o.AuthPrincipal
}

// SetAuthPrincipal gets a reference to the given string and assigns it to the AuthPrincipal field.
func (o *LogExportClusterSpecification) SetAuthPrincipal(v string) {
	o.AuthPrincipal = &v
}

// GetGroups returns the Groups field value if set, zero value otherwise.
func (o *LogExportClusterSpecification) GetGroups() []LogExportGroup {
	if o == nil || o.Groups == nil {
		var ret []LogExportGroup
		return ret
	}
	return *o.Groups
}

// SetGroups gets a reference to the given []LogExportGroup and assigns it to the Groups field.
func (o *LogExportClusterSpecification) SetGroups(v []LogExportGroup) {
	o.Groups = &v
}

// GetLogName returns the LogName field value if set, zero value otherwise.
func (o *LogExportClusterSpecification) GetLogName() string {
	if o == nil || o.LogName == nil {
		var ret string
		return ret
	}
	return *o.LogName
}

// SetLogName gets a reference to the given string and assigns it to the LogName field.
func (o *LogExportClusterSpecification) SetLogName(v string) {
	o.LogName = &v
}

// GetRedact returns the Redact field value if set, zero value otherwise.
func (o *LogExportClusterSpecification) GetRedact() bool {
	if o == nil || o.Redact == nil {
		var ret bool
		return ret
	}
	return *o.Redact
}

// SetRedact gets a reference to the given bool and assigns it to the Redact field.
func (o *LogExportClusterSpecification) SetRedact(v bool) {
	o.Redact = &v
}

// GetRegion returns the Region field value if set, zero value otherwise.
func (o *LogExportClusterSpecification) GetRegion() string {
	if o == nil || o.Region == nil {
		var ret string
		return ret
	}
	return *o.Region
}

// SetRegion gets a reference to the given string and assigns it to the Region field.
func (o *LogExportClusterSpecification) SetRegion(v string) {
	o.Region = &v
}

// GetType returns the Type field value if set, zero value otherwise.
func (o *LogExportClusterSpecification) GetType() LogExportType {
	if o == nil || o.Type == nil {
		var ret LogExportType
		return ret
	}
	return *o.Type
}

// SetType gets a reference to the given LogExportType and assigns it to the Type field.
func (o *LogExportClusterSpecification) SetType(v LogExportType) {
	o.Type = &v
}

func (o LogExportClusterSpecification) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if o.AuthPrincipal != nil {
		toSerialize["auth_principal"] = o.AuthPrincipal
	}
	if o.Groups != nil {
		toSerialize["groups"] = o.Groups
	}
	if o.LogName != nil {
		toSerialize["log_name"] = o.LogName
	}
	if o.Redact != nil {
		toSerialize["redact"] = o.Redact
	}
	if o.Region != nil {
		toSerialize["region"] = o.Region
	}
	if o.Type != nil {
		toSerialize["type"] = o.Type
	}
	return json.Marshal(toSerialize)
}