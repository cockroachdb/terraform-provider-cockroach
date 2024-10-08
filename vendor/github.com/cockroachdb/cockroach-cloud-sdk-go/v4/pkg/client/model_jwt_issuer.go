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

// JWTIssuer struct for JWTIssuer.
type JWTIssuer struct {
	Audience    string                       `json:"audience"`
	Claim       *string                      `json:"claim,omitempty"`
	Id          string                       `json:"id"`
	IdentityMap *[]JWTIssuerIdentityMapEntry `json:"identity_map,omitempty"`
	IssuerUrl   string                       `json:"issuer_url"`
	Jwks        *string                      `json:"jwks,omitempty"`
}

// NewJWTIssuer instantiates a new JWTIssuer object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewJWTIssuer(audience string, id string, issuerUrl string) *JWTIssuer {
	p := JWTIssuer{}
	p.Audience = audience
	p.Id = id
	p.IssuerUrl = issuerUrl
	return &p
}

// NewJWTIssuerWithDefaults instantiates a new JWTIssuer object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewJWTIssuerWithDefaults() *JWTIssuer {
	p := JWTIssuer{}
	return &p
}

// GetAudience returns the Audience field value.
func (o *JWTIssuer) GetAudience() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Audience
}

// SetAudience sets field value.
func (o *JWTIssuer) SetAudience(v string) {
	o.Audience = v
}

// GetClaim returns the Claim field value if set, zero value otherwise.
func (o *JWTIssuer) GetClaim() string {
	if o == nil || o.Claim == nil {
		var ret string
		return ret
	}
	return *o.Claim
}

// SetClaim gets a reference to the given string and assigns it to the Claim field.
func (o *JWTIssuer) SetClaim(v string) {
	o.Claim = &v
}

// GetId returns the Id field value.
func (o *JWTIssuer) GetId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Id
}

// SetId sets field value.
func (o *JWTIssuer) SetId(v string) {
	o.Id = v
}

// GetIdentityMap returns the IdentityMap field value if set, zero value otherwise.
func (o *JWTIssuer) GetIdentityMap() []JWTIssuerIdentityMapEntry {
	if o == nil || o.IdentityMap == nil {
		var ret []JWTIssuerIdentityMapEntry
		return ret
	}
	return *o.IdentityMap
}

// SetIdentityMap gets a reference to the given []JWTIssuerIdentityMapEntry and assigns it to the IdentityMap field.
func (o *JWTIssuer) SetIdentityMap(v []JWTIssuerIdentityMapEntry) {
	o.IdentityMap = &v
}

// GetIssuerUrl returns the IssuerUrl field value.
func (o *JWTIssuer) GetIssuerUrl() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.IssuerUrl
}

// SetIssuerUrl sets field value.
func (o *JWTIssuer) SetIssuerUrl(v string) {
	o.IssuerUrl = v
}

// GetJwks returns the Jwks field value if set, zero value otherwise.
func (o *JWTIssuer) GetJwks() string {
	if o == nil || o.Jwks == nil {
		var ret string
		return ret
	}
	return *o.Jwks
}

// SetJwks gets a reference to the given string and assigns it to the Jwks field.
func (o *JWTIssuer) SetJwks(v string) {
	o.Jwks = &v
}
