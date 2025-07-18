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
	"time"
)

// PhysicalReplicationStream struct for PhysicalReplicationStream.
type PhysicalReplicationStream struct {
	// activated_at is the crdb system time at which failover is finalized. This may differ from the time for which failover was requested. This field will be present when a replication stream is in the COMPLETED state.
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
	// canceled_at is the timestamp at which the replication stream was canceled.
	CanceledAt *time.Time `json:"canceled_at,omitempty"`
	// created_at is the timestamp at which the replication stream was created.
	CreatedAt time.Time `json:"created_at"`
	// failover_at is the time for which failover is requested. If the user sets the status to 'FAILING_OVER' but omits failover_at, the failover time will default to the latest consistent replicated time. Otherwise, the user can pick a time up to one hour in the future to schedule a failover, or a time in the past to restore the cluster to a recent state. This field will be present if the user has requested failover at a future time.
	FailoverAt *time.Time `json:"failover_at,omitempty"`
	// id is the UUID of the replication stream.
	Id string `json:"id"`
	// primary_cluster_id is the ID of the cluster that is being replicated.
	PrimaryClusterId string `json:"primary_cluster_id"`
	// replicated_time is the timestamp indicating the point up to which data has been replicated. The window between replicated_time and the actual time is known as replication lag. This field will be present when a replication stream is in the REPLICATING state.
	ReplicatedTime *time.Time `json:"replicated_time,omitempty"`
	// replication_lag_seconds is the replication lag (current time minus replicated time) in seconds. This field will be present when a replication stream is in the REPLICATING state.
	ReplicationLagSeconds *int32 `json:"replication_lag_seconds,omitempty"`
	// retained_time is the timestamp indicating the lower bound that the replication stream can failover to. This field will be present when a replication stream is in the REPLICATING state.
	RetainedTime *time.Time `json:"retained_time,omitempty"`
	// standby_cluster_id is the ID of the standby cluster that data is being replicated to.
	StandbyClusterId string                      `json:"standby_cluster_id"`
	Status           ReplicationStreamStatusType `json:"status"`
}

// NewPhysicalReplicationStream instantiates a new PhysicalReplicationStream object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewPhysicalReplicationStream(createdAt time.Time, id string, primaryClusterId string, standbyClusterId string, status ReplicationStreamStatusType) *PhysicalReplicationStream {
	p := PhysicalReplicationStream{}
	p.CreatedAt = createdAt
	p.Id = id
	p.PrimaryClusterId = primaryClusterId
	p.StandbyClusterId = standbyClusterId
	p.Status = status
	return &p
}

// NewPhysicalReplicationStreamWithDefaults instantiates a new PhysicalReplicationStream object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewPhysicalReplicationStreamWithDefaults() *PhysicalReplicationStream {
	p := PhysicalReplicationStream{}
	return &p
}

// GetActivatedAt returns the ActivatedAt field value if set, zero value otherwise.
func (o *PhysicalReplicationStream) GetActivatedAt() time.Time {
	if o == nil || o.ActivatedAt == nil {
		var ret time.Time
		return ret
	}
	return *o.ActivatedAt
}

// SetActivatedAt gets a reference to the given time.Time and assigns it to the ActivatedAt field.
func (o *PhysicalReplicationStream) SetActivatedAt(v time.Time) {
	o.ActivatedAt = &v
}

// GetCanceledAt returns the CanceledAt field value if set, zero value otherwise.
func (o *PhysicalReplicationStream) GetCanceledAt() time.Time {
	if o == nil || o.CanceledAt == nil {
		var ret time.Time
		return ret
	}
	return *o.CanceledAt
}

// SetCanceledAt gets a reference to the given time.Time and assigns it to the CanceledAt field.
func (o *PhysicalReplicationStream) SetCanceledAt(v time.Time) {
	o.CanceledAt = &v
}

// GetCreatedAt returns the CreatedAt field value.
func (o *PhysicalReplicationStream) GetCreatedAt() time.Time {
	if o == nil {
		var ret time.Time
		return ret
	}

	return o.CreatedAt
}

// SetCreatedAt sets field value.
func (o *PhysicalReplicationStream) SetCreatedAt(v time.Time) {
	o.CreatedAt = v
}

// GetFailoverAt returns the FailoverAt field value if set, zero value otherwise.
func (o *PhysicalReplicationStream) GetFailoverAt() time.Time {
	if o == nil || o.FailoverAt == nil {
		var ret time.Time
		return ret
	}
	return *o.FailoverAt
}

// SetFailoverAt gets a reference to the given time.Time and assigns it to the FailoverAt field.
func (o *PhysicalReplicationStream) SetFailoverAt(v time.Time) {
	o.FailoverAt = &v
}

// GetId returns the Id field value.
func (o *PhysicalReplicationStream) GetId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Id
}

// SetId sets field value.
func (o *PhysicalReplicationStream) SetId(v string) {
	o.Id = v
}

// GetPrimaryClusterId returns the PrimaryClusterId field value.
func (o *PhysicalReplicationStream) GetPrimaryClusterId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.PrimaryClusterId
}

// SetPrimaryClusterId sets field value.
func (o *PhysicalReplicationStream) SetPrimaryClusterId(v string) {
	o.PrimaryClusterId = v
}

// GetReplicatedTime returns the ReplicatedTime field value if set, zero value otherwise.
func (o *PhysicalReplicationStream) GetReplicatedTime() time.Time {
	if o == nil || o.ReplicatedTime == nil {
		var ret time.Time
		return ret
	}
	return *o.ReplicatedTime
}

// SetReplicatedTime gets a reference to the given time.Time and assigns it to the ReplicatedTime field.
func (o *PhysicalReplicationStream) SetReplicatedTime(v time.Time) {
	o.ReplicatedTime = &v
}

// GetReplicationLagSeconds returns the ReplicationLagSeconds field value if set, zero value otherwise.
func (o *PhysicalReplicationStream) GetReplicationLagSeconds() int32 {
	if o == nil || o.ReplicationLagSeconds == nil {
		var ret int32
		return ret
	}
	return *o.ReplicationLagSeconds
}

// SetReplicationLagSeconds gets a reference to the given int32 and assigns it to the ReplicationLagSeconds field.
func (o *PhysicalReplicationStream) SetReplicationLagSeconds(v int32) {
	o.ReplicationLagSeconds = &v
}

// GetRetainedTime returns the RetainedTime field value if set, zero value otherwise.
func (o *PhysicalReplicationStream) GetRetainedTime() time.Time {
	if o == nil || o.RetainedTime == nil {
		var ret time.Time
		return ret
	}
	return *o.RetainedTime
}

// SetRetainedTime gets a reference to the given time.Time and assigns it to the RetainedTime field.
func (o *PhysicalReplicationStream) SetRetainedTime(v time.Time) {
	o.RetainedTime = &v
}

// GetStandbyClusterId returns the StandbyClusterId field value.
func (o *PhysicalReplicationStream) GetStandbyClusterId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.StandbyClusterId
}

// SetStandbyClusterId sets field value.
func (o *PhysicalReplicationStream) SetStandbyClusterId(v string) {
	o.StandbyClusterId = v
}

// GetStatus returns the Status field value.
func (o *PhysicalReplicationStream) GetStatus() ReplicationStreamStatusType {
	if o == nil {
		var ret ReplicationStreamStatusType
		return ret
	}

	return o.Status
}

// SetStatus sets field value.
func (o *PhysicalReplicationStream) SetStatus(v ReplicationStreamStatusType) {
	o.Status = v
}
