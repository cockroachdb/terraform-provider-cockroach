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

import (
	"fmt"
)

// ReplicationStreamStatusType the model 'ReplicationStreamStatusType'.
type ReplicationStreamStatusType string

// List of ReplicationStreamStatus.Type.
const (
	REPLICATIONSTREAMSTATUSTYPE_STARTING     ReplicationStreamStatusType = "STARTING"
	REPLICATIONSTREAMSTATUSTYPE_REPLICATING  ReplicationStreamStatusType = "REPLICATING"
	REPLICATIONSTREAMSTATUSTYPE_FAILING_OVER ReplicationStreamStatusType = "FAILING_OVER"
	REPLICATIONSTREAMSTATUSTYPE_COMPLETED    ReplicationStreamStatusType = "COMPLETED"
)

// All allowed values of ReplicationStreamStatusType enum.
var AllowedReplicationStreamStatusTypeEnumValues = []ReplicationStreamStatusType{
	"STARTING",
	"REPLICATING",
	"FAILING_OVER",
	"COMPLETED",
}

// NewReplicationStreamStatusTypeFromValue returns a pointer to a valid ReplicationStreamStatusType
// for the value passed as argument, or an error if the value passed is not allowed by the enum.
func NewReplicationStreamStatusTypeFromValue(v string) (*ReplicationStreamStatusType, error) {
	ev := ReplicationStreamStatusType(v)
	if ev.IsValid() {
		return &ev, nil
	} else {
		return nil, fmt.Errorf("invalid value '%v' for ReplicationStreamStatusType: valid values are %v", v, AllowedReplicationStreamStatusTypeEnumValues)
	}
}

// IsValid return true if the value is valid for the enum, false otherwise.
func (v ReplicationStreamStatusType) IsValid() bool {
	for _, existing := range AllowedReplicationStreamStatusTypeEnumValues {
		if existing == v {
			return true
		}
	}
	return false
}

// Ptr returns reference to ReplicationStreamStatus.Type value.
func (v ReplicationStreamStatusType) Ptr() *ReplicationStreamStatusType {
	return &v
}
