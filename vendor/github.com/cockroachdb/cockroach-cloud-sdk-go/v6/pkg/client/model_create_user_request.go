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

// CreateUserRequest struct for CreateUserRequest.
type CreateUserRequest struct {
	Active      *bool       `json:"active,omitempty"`
	DisplayName *string     `json:"displayName,omitempty"`
	Emails      []ScimEmail `json:"emails"`
	ExternalId  *string     `json:"externalId,omitempty"`
	Name        *ScimName   `json:"name,omitempty"`
	Schemas     []string    `json:"schemas"`
	UserName    *string     `json:"userName,omitempty"`
}

// NewCreateUserRequest instantiates a new CreateUserRequest object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewCreateUserRequest(emails []ScimEmail, schemas []string) *CreateUserRequest {
	p := CreateUserRequest{}
	p.Emails = emails
	p.Schemas = schemas
	return &p
}

// NewCreateUserRequestWithDefaults instantiates a new CreateUserRequest object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewCreateUserRequestWithDefaults() *CreateUserRequest {
	p := CreateUserRequest{}
	return &p
}

// GetActive returns the Active field value if set, zero value otherwise.
func (o *CreateUserRequest) GetActive() bool {
	if o == nil || o.Active == nil {
		var ret bool
		return ret
	}
	return *o.Active
}

// SetActive gets a reference to the given bool and assigns it to the Active field.
func (o *CreateUserRequest) SetActive(v bool) {
	o.Active = &v
}

// GetDisplayName returns the DisplayName field value if set, zero value otherwise.
func (o *CreateUserRequest) GetDisplayName() string {
	if o == nil || o.DisplayName == nil {
		var ret string
		return ret
	}
	return *o.DisplayName
}

// SetDisplayName gets a reference to the given string and assigns it to the DisplayName field.
func (o *CreateUserRequest) SetDisplayName(v string) {
	o.DisplayName = &v
}

// GetEmails returns the Emails field value.
func (o *CreateUserRequest) GetEmails() []ScimEmail {
	if o == nil {
		var ret []ScimEmail
		return ret
	}

	return o.Emails
}

// SetEmails sets field value.
func (o *CreateUserRequest) SetEmails(v []ScimEmail) {
	o.Emails = v
}

// GetExternalId returns the ExternalId field value if set, zero value otherwise.
func (o *CreateUserRequest) GetExternalId() string {
	if o == nil || o.ExternalId == nil {
		var ret string
		return ret
	}
	return *o.ExternalId
}

// SetExternalId gets a reference to the given string and assigns it to the ExternalId field.
func (o *CreateUserRequest) SetExternalId(v string) {
	o.ExternalId = &v
}

// GetName returns the Name field value if set, zero value otherwise.
func (o *CreateUserRequest) GetName() ScimName {
	if o == nil || o.Name == nil {
		var ret ScimName
		return ret
	}
	return *o.Name
}

// SetName gets a reference to the given ScimName and assigns it to the Name field.
func (o *CreateUserRequest) SetName(v ScimName) {
	o.Name = &v
}

// GetSchemas returns the Schemas field value.
func (o *CreateUserRequest) GetSchemas() []string {
	if o == nil {
		var ret []string
		return ret
	}

	return o.Schemas
}

// SetSchemas sets field value.
func (o *CreateUserRequest) SetSchemas(v []string) {
	o.Schemas = v
}

// GetUserName returns the UserName field value if set, zero value otherwise.
func (o *CreateUserRequest) GetUserName() string {
	if o == nil || o.UserName == nil {
		var ret string
		return ret
	}
	return *o.UserName
}

// SetUserName gets a reference to the given string and assigns it to the UserName field.
func (o *CreateUserRequest) SetUserName(v string) {
	o.UserName = &v
}
