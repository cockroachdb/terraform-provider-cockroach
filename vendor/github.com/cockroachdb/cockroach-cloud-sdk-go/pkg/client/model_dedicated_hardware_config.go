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

// DedicatedHardwareConfig struct for DedicatedHardwareConfig.
type DedicatedHardwareConfig struct {
	// disk_iops is the number of disk I/O operations per second that are permitted on each node in the cluster. Zero indicates the cloud provider-specific default.
	DiskIops int32 `json:"disk_iops"`
	// machine_type is the machine type identifier within the given cloud provider, ex. m5.xlarge, n2-standard-4.
	MachineType string `json:"machine_type"`
	// memory_gib is the memory GiB per node in the cluster.
	MemoryGib float32 `json:"memory_gib"`
	// num_virtual_cpus is the number of virtual CPUs per node in the cluster.
	NumVirtualCpus int32 `json:"num_virtual_cpus"`
	// storage_gib is the number of storage GiB per node in the cluster.
	StorageGib int32 `json:"storage_gib"`
}

// NewDedicatedHardwareConfig instantiates a new DedicatedHardwareConfig object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewDedicatedHardwareConfig(diskIops int32, machineType string, memoryGib float32, numVirtualCpus int32, storageGib int32) *DedicatedHardwareConfig {
	p := DedicatedHardwareConfig{}
	p.DiskIops = diskIops
	p.MachineType = machineType
	p.MemoryGib = memoryGib
	p.NumVirtualCpus = numVirtualCpus
	p.StorageGib = storageGib
	return &p
}

// NewDedicatedHardwareConfigWithDefaults instantiates a new DedicatedHardwareConfig object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewDedicatedHardwareConfigWithDefaults() *DedicatedHardwareConfig {
	p := DedicatedHardwareConfig{}
	return &p
}

// GetDiskIops returns the DiskIops field value.
func (o *DedicatedHardwareConfig) GetDiskIops() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.DiskIops
}

// SetDiskIops sets field value.
func (o *DedicatedHardwareConfig) SetDiskIops(v int32) {
	o.DiskIops = v
}

// GetMachineType returns the MachineType field value.
func (o *DedicatedHardwareConfig) GetMachineType() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.MachineType
}

// SetMachineType sets field value.
func (o *DedicatedHardwareConfig) SetMachineType(v string) {
	o.MachineType = v
}

// GetMemoryGib returns the MemoryGib field value.
func (o *DedicatedHardwareConfig) GetMemoryGib() float32 {
	if o == nil {
		var ret float32
		return ret
	}

	return o.MemoryGib
}

// SetMemoryGib sets field value.
func (o *DedicatedHardwareConfig) SetMemoryGib(v float32) {
	o.MemoryGib = v
}

// GetNumVirtualCpus returns the NumVirtualCpus field value.
func (o *DedicatedHardwareConfig) GetNumVirtualCpus() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.NumVirtualCpus
}

// SetNumVirtualCpus sets field value.
func (o *DedicatedHardwareConfig) SetNumVirtualCpus(v int32) {
	o.NumVirtualCpus = v
}

// GetStorageGib returns the StorageGib field value.
func (o *DedicatedHardwareConfig) GetStorageGib() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.StorageGib
}

// SetStorageGib sets field value.
func (o *DedicatedHardwareConfig) SetStorageGib(v int32) {
	o.StorageGib = v
}
