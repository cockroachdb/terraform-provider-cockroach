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

// EnableDatadogMetricExportRequest struct for EnableDatadogMetricExportRequest.
type EnableDatadogMetricExportRequest struct {
	// api_key is a Datadog API key.
	ApiKey string          `json:"api_key"`
	Site   DatadogSiteType `json:"site"`
}

// NewEnableDatadogMetricExportRequest instantiates a new EnableDatadogMetricExportRequest object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewEnableDatadogMetricExportRequest(apiKey string, site DatadogSiteType) *EnableDatadogMetricExportRequest {
	p := EnableDatadogMetricExportRequest{}
	p.ApiKey = apiKey
	p.Site = site
	return &p
}

// NewEnableDatadogMetricExportRequestWithDefaults instantiates a new EnableDatadogMetricExportRequest object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewEnableDatadogMetricExportRequestWithDefaults() *EnableDatadogMetricExportRequest {
	p := EnableDatadogMetricExportRequest{}
	return &p
}

// GetApiKey returns the ApiKey field value.
func (o *EnableDatadogMetricExportRequest) GetApiKey() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.ApiKey
}

// SetApiKey sets field value.
func (o *EnableDatadogMetricExportRequest) SetApiKey(v string) {
	o.ApiKey = v
}

// GetSite returns the Site field value.
func (o *EnableDatadogMetricExportRequest) GetSite() DatadogSiteType {
	if o == nil {
		var ret DatadogSiteType
		return ret
	}

	return o.Site
}

// SetSite sets field value.
func (o *EnableDatadogMetricExportRequest) SetSite(v DatadogSiteType) {
	o.Site = v
}
