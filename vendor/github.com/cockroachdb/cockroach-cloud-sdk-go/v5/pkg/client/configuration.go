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
	"net/http"
)

const DefaultServerURL string = "https://cockroachlabs.cloud"

const ApiVersion = "2024-09-16"

// Configuration stores the configuration of the API client.
type Configuration struct {
	Host          string            `json:"host,omitempty"`
	Scheme        string            `json:"scheme,omitempty"`
	DefaultHeader map[string]string `json:"defaultHeader,omitempty"`
	UserAgent     string            `json:"userAgent,omitempty"`
	Debug         bool              `json:"debug,omitempty"`
	ServerURL     string
	HTTPClient    *http.Client
	apiToken      string
}

// ConfigurationOption is a function that sets some configuration options.
type ConfigurationOption func(*Configuration)

// WithVanityName sets the vanity name in the request header.
func WithVanityName(vanityName string) ConfigurationOption {
	return func(cfg *Configuration) {
		cfg.AddDefaultHeader("Cc-Vanity-Name", vanityName)
	}
}

// WithUsername sets the username in the request header.
func WithUsername(username string) ConfigurationOption {
	return func(cfg *Configuration) {
		cfg.AddDefaultHeader("Cc-Username", username)
	}
}

// NewConfiguration returns a new Configuration object.
// The apiToken is a secret that is used to authenticate with the API.
// It is either the API Key from a Service Account or a JWT from a JWT Issuer
// configured for the CockroachDB Cloud Organization.
// In the case of JWT, the vanity name is required and can be provided using the
// WithVanityName option.
func NewConfiguration(apiToken string, opts ...ConfigurationOption) *Configuration {
	cfg := &Configuration{
		DefaultHeader: make(map[string]string),
		UserAgent:     "ccloud-sdk-go/5.1.0",
		Debug:         false,
		ServerURL:     DefaultServerURL,
		apiToken:      apiToken,
	}

	cfg.AddDefaultHeader("Cc-Version", ApiVersion)

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// AddDefaultHeader adds a new HTTP header to the default header in the request.
func (c *Configuration) AddDefaultHeader(key string, value string) {
	c.DefaultHeader[key] = value
}
