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

// CloudWatchMetricExportInfo struct for CloudWatchMetricExportInfo.
type CloudWatchMetricExportInfo struct {
	ClusterId string `json:"cluster_id"`
	// log_group_name is the customized log group name.
	LogGroupName *string `json:"log_group_name,omitempty"`
	// role_arn is the IAM role used to upload metric segments to the target AWS account.
	RoleArn string                  `json:"role_arn"`
	Status  *MetricExportStatusType `json:"status,omitempty"`
	// target_region specifies the specific AWS region that the metrics will be exported to.
	TargetRegion *string `json:"target_region,omitempty"`
	UserMessage  *string `json:"user_message,omitempty"`
}

// NewCloudWatchMetricExportInfo instantiates a new CloudWatchMetricExportInfo object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewCloudWatchMetricExportInfo(clusterId string, roleArn string) *CloudWatchMetricExportInfo {
	p := CloudWatchMetricExportInfo{}
	p.ClusterId = clusterId
	p.RoleArn = roleArn
	return &p
}

// NewCloudWatchMetricExportInfoWithDefaults instantiates a new CloudWatchMetricExportInfo object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewCloudWatchMetricExportInfoWithDefaults() *CloudWatchMetricExportInfo {
	p := CloudWatchMetricExportInfo{}
	return &p
}

// GetClusterId returns the ClusterId field value.
func (o *CloudWatchMetricExportInfo) GetClusterId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.ClusterId
}

// SetClusterId sets field value.
func (o *CloudWatchMetricExportInfo) SetClusterId(v string) {
	o.ClusterId = v
}

// GetLogGroupName returns the LogGroupName field value if set, zero value otherwise.
func (o *CloudWatchMetricExportInfo) GetLogGroupName() string {
	if o == nil || o.LogGroupName == nil {
		var ret string
		return ret
	}
	return *o.LogGroupName
}

// SetLogGroupName gets a reference to the given string and assigns it to the LogGroupName field.
func (o *CloudWatchMetricExportInfo) SetLogGroupName(v string) {
	o.LogGroupName = &v
}

// GetRoleArn returns the RoleArn field value.
func (o *CloudWatchMetricExportInfo) GetRoleArn() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.RoleArn
}

// SetRoleArn sets field value.
func (o *CloudWatchMetricExportInfo) SetRoleArn(v string) {
	o.RoleArn = v
}

// GetStatus returns the Status field value if set, zero value otherwise.
func (o *CloudWatchMetricExportInfo) GetStatus() MetricExportStatusType {
	if o == nil || o.Status == nil {
		var ret MetricExportStatusType
		return ret
	}
	return *o.Status
}

// SetStatus gets a reference to the given MetricExportStatusType and assigns it to the Status field.
func (o *CloudWatchMetricExportInfo) SetStatus(v MetricExportStatusType) {
	o.Status = &v
}

// GetTargetRegion returns the TargetRegion field value if set, zero value otherwise.
func (o *CloudWatchMetricExportInfo) GetTargetRegion() string {
	if o == nil || o.TargetRegion == nil {
		var ret string
		return ret
	}
	return *o.TargetRegion
}

// SetTargetRegion gets a reference to the given string and assigns it to the TargetRegion field.
func (o *CloudWatchMetricExportInfo) SetTargetRegion(v string) {
	o.TargetRegion = &v
}

// GetUserMessage returns the UserMessage field value if set, zero value otherwise.
func (o *CloudWatchMetricExportInfo) GetUserMessage() string {
	if o == nil || o.UserMessage == nil {
		var ret string
		return ret
	}
	return *o.UserMessage
}

// SetUserMessage gets a reference to the given string and assigns it to the UserMessage field.
func (o *CloudWatchMetricExportInfo) SetUserMessage(v string) {
	o.UserMessage = &v
}

func (o CloudWatchMetricExportInfo) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["cluster_id"] = o.ClusterId
	}
	if o.LogGroupName != nil {
		toSerialize["log_group_name"] = o.LogGroupName
	}
	if true {
		toSerialize["role_arn"] = o.RoleArn
	}
	if o.Status != nil {
		toSerialize["status"] = o.Status
	}
	if o.TargetRegion != nil {
		toSerialize["target_region"] = o.TargetRegion
	}
	if o.UserMessage != nil {
		toSerialize["user_message"] = o.UserMessage
	}
	return json.Marshal(toSerialize)
}
