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

// AddPrivateEndpointTrustedOwnerRequest struct for AddPrivateEndpointTrustedOwnerRequest.
type AddPrivateEndpointTrustedOwnerRequest struct {
	// external_owner_id is the identifier of the owner within the cloud provider for private endpoint connections. A wildcard character (\"*\") can be used to denote all owners.
	ExternalOwnerId string                              `json:"external_owner_id"`
	Type            PrivateEndpointTrustedOwnerTypeType `json:"type"`
}

// NewAddPrivateEndpointTrustedOwnerRequest instantiates a new AddPrivateEndpointTrustedOwnerRequest object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewAddPrivateEndpointTrustedOwnerRequest(externalOwnerId string, type_ PrivateEndpointTrustedOwnerTypeType) *AddPrivateEndpointTrustedOwnerRequest {
	p := AddPrivateEndpointTrustedOwnerRequest{}
	p.ExternalOwnerId = externalOwnerId
	p.Type = type_
	return &p
}

// NewAddPrivateEndpointTrustedOwnerRequestWithDefaults instantiates a new AddPrivateEndpointTrustedOwnerRequest object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewAddPrivateEndpointTrustedOwnerRequestWithDefaults() *AddPrivateEndpointTrustedOwnerRequest {
	p := AddPrivateEndpointTrustedOwnerRequest{}
	return &p
}

// GetExternalOwnerId returns the ExternalOwnerId field value.
func (o *AddPrivateEndpointTrustedOwnerRequest) GetExternalOwnerId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.ExternalOwnerId
}

// SetExternalOwnerId sets field value.
func (o *AddPrivateEndpointTrustedOwnerRequest) SetExternalOwnerId(v string) {
	o.ExternalOwnerId = v
}

// GetType returns the Type field value.
func (o *AddPrivateEndpointTrustedOwnerRequest) GetType() PrivateEndpointTrustedOwnerTypeType {
	if o == nil {
		var ret PrivateEndpointTrustedOwnerTypeType
		return ret
	}

	return o.Type
}

// SetType sets field value.
func (o *AddPrivateEndpointTrustedOwnerRequest) SetType(v PrivateEndpointTrustedOwnerTypeType) {
	o.Type = v
}
