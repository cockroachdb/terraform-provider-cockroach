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

// DeleteMetricExportResponse struct for DeleteMetricExportResponse.
type DeleteMetricExportResponse struct {
	ClusterId string              `json:"cluster_id"`
	Status    *MetricExportStatus `json:"status,omitempty"`
}

// NewDeleteMetricExportResponse instantiates a new DeleteMetricExportResponse object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewDeleteMetricExportResponse(clusterId string) *DeleteMetricExportResponse {
	p := DeleteMetricExportResponse{}
	p.ClusterId = clusterId
	return &p
}

// NewDeleteMetricExportResponseWithDefaults instantiates a new DeleteMetricExportResponse object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewDeleteMetricExportResponseWithDefaults() *DeleteMetricExportResponse {
	p := DeleteMetricExportResponse{}
	return &p
}

// GetClusterId returns the ClusterId field value.
func (o *DeleteMetricExportResponse) GetClusterId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.ClusterId
}

// SetClusterId sets field value.
func (o *DeleteMetricExportResponse) SetClusterId(v string) {
	o.ClusterId = v
}

// GetStatus returns the Status field value if set, zero value otherwise.
func (o *DeleteMetricExportResponse) GetStatus() MetricExportStatus {
	if o == nil || o.Status == nil {
		var ret MetricExportStatus
		return ret
	}
	return *o.Status
}

// SetStatus gets a reference to the given MetricExportStatus and assigns it to the Status field.
func (o *DeleteMetricExportResponse) SetStatus(v MetricExportStatus) {
	o.Status = &v
}

func (o DeleteMetricExportResponse) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["cluster_id"] = o.ClusterId
	}
	if o.Status != nil {
		toSerialize["status"] = o.Status
	}
	return json.Marshal(toSerialize)
}
