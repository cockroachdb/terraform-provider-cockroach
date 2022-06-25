// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.
// CockroachDB Cloud API
// API version: 2022-03-31

package client

import (
	"encoding/json"
)

// DedicatedHardwareCreateSpecification struct for DedicatedHardwareCreateSpecification.
type DedicatedHardwareCreateSpecification struct {
	MachineSpec DedicatedMachineTypeSpecification `json:"machine_spec"`
	// StorageGiB is the number of storage GiB per node in the cluster. Zero indicates default to the lowest storage GiB available given machine specs.
	StorageGib int32 `json:"storage_gib"`
	// DiskIOPs is the number of disk I/O operations per second that are permitted on each node in the cluster. Zero indicates the cloud provider-specific default. Only available for AWS clusters.
	DiskIops             *int32 `json:"disk_iops,omitempty"`
	AdditionalProperties map[string]interface{}
}

type dedicatedHardwareCreateSpecification DedicatedHardwareCreateSpecification

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

func (o DedicatedHardwareCreateSpecification) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["machine_spec"] = o.MachineSpec
	}
	if true {
		toSerialize["storage_gib"] = o.StorageGib
	}
	if o.DiskIops != nil {
		toSerialize["disk_iops"] = o.DiskIops
	}

	for key, value := range o.AdditionalProperties {
		toSerialize[key] = value
	}

	return json.Marshal(toSerialize)
}

func (o *DedicatedHardwareCreateSpecification) UnmarshalJSON(bytes []byte) (err error) {
	varDedicatedHardwareCreateSpecification := dedicatedHardwareCreateSpecification{}

	if err = json.Unmarshal(bytes, &varDedicatedHardwareCreateSpecification); err == nil {
		*o = DedicatedHardwareCreateSpecification(varDedicatedHardwareCreateSpecification)
	}

	additionalProperties := make(map[string]interface{})

	if err = json.Unmarshal(bytes, &additionalProperties); err == nil {
		delete(additionalProperties, "machine_spec")
		delete(additionalProperties, "storage_gib")
		delete(additionalProperties, "disk_iops")
		o.AdditionalProperties = additionalProperties
	}

	return err
}