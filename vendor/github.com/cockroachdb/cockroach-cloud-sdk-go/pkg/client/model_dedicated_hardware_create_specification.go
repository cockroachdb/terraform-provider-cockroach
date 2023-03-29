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

// DedicatedHardwareCreateSpecification struct for DedicatedHardwareCreateSpecification.
type DedicatedHardwareCreateSpecification struct {
	// disk_iops is the number of disk I/O operations per second that are permitted on each node in the cluster. Zero indicates the cloud provider-specific default. Only available for AWS clusters.
	DiskIops    *int32                            `json:"disk_iops,omitempty"`
	MachineSpec DedicatedMachineTypeSpecification `json:"machine_spec"`
	// storage_gib is the number of storage GiB per node in the cluster. Zero indicates default to the lowest storage GiB available given machine specs.
	StorageGib int32 `json:"storage_gib"`
}

// NewDedicatedHardwareCreateSpecification instantiates a new DedicatedHardwareCreateSpecification object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewDedicatedHardwareCreateSpecification(machineSpec DedicatedMachineTypeSpecification, storageGib int32) *DedicatedHardwareCreateSpecification {
	p := DedicatedHardwareCreateSpecification{}
	p.MachineSpec = machineSpec
	p.StorageGib = storageGib
	return &p
}

// NewDedicatedHardwareCreateSpecificationWithDefaults instantiates a new DedicatedHardwareCreateSpecification object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewDedicatedHardwareCreateSpecificationWithDefaults() *DedicatedHardwareCreateSpecification {
	p := DedicatedHardwareCreateSpecification{}
	return &p
}

// GetDiskIops returns the DiskIops field value if set, zero value otherwise.
func (o *DedicatedHardwareCreateSpecification) GetDiskIops() int32 {
	if o == nil || o.DiskIops == nil {
		var ret int32
		return ret
	}
	return *o.DiskIops
}

// SetDiskIops gets a reference to the given int32 and assigns it to the DiskIops field.
func (o *DedicatedHardwareCreateSpecification) SetDiskIops(v int32) {
	o.DiskIops = &v
}

// GetMachineSpec returns the MachineSpec field value.
func (o *DedicatedHardwareCreateSpecification) GetMachineSpec() DedicatedMachineTypeSpecification {
	if o == nil {
		var ret DedicatedMachineTypeSpecification
		return ret
	}

	return o.MachineSpec
}

// SetMachineSpec sets field value.
func (o *DedicatedHardwareCreateSpecification) SetMachineSpec(v DedicatedMachineTypeSpecification) {
	o.MachineSpec = v
}

// GetStorageGib returns the StorageGib field value.
func (o *DedicatedHardwareCreateSpecification) GetStorageGib() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.StorageGib
}

// SetStorageGib sets field value.
func (o *DedicatedHardwareCreateSpecification) SetStorageGib(v int32) {
	o.StorageGib = v
}

func (o DedicatedHardwareCreateSpecification) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if o.DiskIops != nil {
		toSerialize["disk_iops"] = o.DiskIops
	}
	if true {
		toSerialize["machine_spec"] = o.MachineSpec
	}
	if true {
		toSerialize["storage_gib"] = o.StorageGib
	}
	return json.Marshal(toSerialize)
}