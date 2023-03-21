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
	"fmt"
)

// NodeStatus the model 'NodeStatus'.
type NodeStatus string

// List of NodeStatus.
const (
	NODESTATUS_LIVE      NodeStatus = "LIVE"
	NODESTATUS_NOT_READY NodeStatus = "NOT_READY"
)

// All allowed values of NodeStatus enum.
var AllowedNodeStatusEnumValues = []NodeStatus{
	"LIVE",
	"NOT_READY",
}

func (v *NodeStatus) UnmarshalJSON(src []byte) error {
	var value string
	err := json.Unmarshal(src, &value)
	if err != nil {
		return err
	}
	enumTypeValue := NodeStatus(value)
	for _, existing := range AllowedNodeStatusEnumValues {
		if existing == enumTypeValue {
			*v = enumTypeValue
			return nil
		}
	}

	return fmt.Errorf("%+v is not a valid NodeStatus", value)
}

// NewNodeStatusFromValue returns a pointer to a valid NodeStatus
// for the value passed as argument, or an error if the value passed is not allowed by the enum.
func NewNodeStatusFromValue(v string) (*NodeStatus, error) {
	ev := NodeStatus(v)
	if ev.IsValid() {
		return &ev, nil
	} else {
		return nil, fmt.Errorf("invalid value '%v' for NodeStatus: valid values are %v", v, AllowedNodeStatusEnumValues)
	}
}

// IsValid return true if the value is valid for the enum, false otherwise.
func (v NodeStatus) IsValid() bool {
	for _, existing := range AllowedNodeStatusEnumValues {
		if existing == v {
			return true
		}
	}
	return false
}

// Ptr returns reference to NodeStatus value.
func (v NodeStatus) Ptr() *NodeStatus {
	return &v
}
