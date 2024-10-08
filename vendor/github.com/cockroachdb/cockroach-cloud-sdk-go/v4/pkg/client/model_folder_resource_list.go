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

// FolderResourceList FolderResourceList contains a list of resources..
type FolderResourceList struct {
	Pagination *KeysetPaginationResponse `json:"pagination,omitempty"`
	Resources  *[]FolderResource         `json:"resources,omitempty"`
}

// NewFolderResourceList instantiates a new FolderResourceList object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewFolderResourceList() *FolderResourceList {
	p := FolderResourceList{}
	return &p
}

// GetPagination returns the Pagination field value if set, zero value otherwise.
func (o *FolderResourceList) GetPagination() KeysetPaginationResponse {
	if o == nil || o.Pagination == nil {
		var ret KeysetPaginationResponse
		return ret
	}
	return *o.Pagination
}

// SetPagination gets a reference to the given KeysetPaginationResponse and assigns it to the Pagination field.
func (o *FolderResourceList) SetPagination(v KeysetPaginationResponse) {
	o.Pagination = &v
}

// GetResources returns the Resources field value if set, zero value otherwise.
func (o *FolderResourceList) GetResources() []FolderResource {
	if o == nil || o.Resources == nil {
		var ret []FolderResource
		return ret
	}
	return *o.Resources
}

// SetResources gets a reference to the given []FolderResource and assigns it to the Resources field.
func (o *FolderResourceList) SetResources(v []FolderResource) {
	o.Resources = &v
}
