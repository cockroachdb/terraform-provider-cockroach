/*
 Copyright 2023 The Cockroach Authors

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package provider

import (
	"context"
	cryptorand "crypto/rand"
	"math/big"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v10/pkg/client"
	tf_provider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"
)

var testAccProvider tf_provider.Provider
var cl *client.Client

func init() {
	apiKey := os.Getenv(CockroachAPIKey)
	apiJWT := os.Getenv(CockroachAPIJWT)

	cfg := getClientConfiguration(apiKey, apiJWT, "test", "")
	cfg.HTTPClient = &http.Client{Transport: recorderTransport{inner: http.DefaultTransport}}

	cl = client.NewClient(cfg)
	testAccProvider = New("test")()
	installRecordingHooks()
}

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"cockroach": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	if mode := recordingMode(); mode != "" && strings.HasPrefix(t.Name(), "TestAcc") {
		if r := currentRecorder(); r == nil || r.t != t {
			t.Skipf("%s=%s, but this test doesn't call accTestSetup", recordingModeEnvVar, mode)
		}
	}
	if os.Getenv(CockroachAPIKey) == "" && os.Getenv(CockroachAPIJWT) == "" {
		t.Fatalf(
			"%s or %s must be set for acceptance testing",
			CockroachAPIKey,
			CockroachAPIJWT,
		)
	}
}

func testAccContinuumPreCheck(t *testing.T) {
	if os.Getenv(cockroachContinuumAPIKey) == "" {
		t.Skipf("%s must be set to run Continuum acceptance tests", cockroachContinuumAPIKey)
	}
}

// continuumProvider authenticates against a Continuum-enabled org using a fixed
// API key. Each continuumProviderFactories call builds its own instance, so a
// parallel Continuum test can't leak its credential into a sibling test's
// provider the way sharing testAccProtoV6ProviderFactories would.
type continuumProvider struct {
	*provider
	apiKey string
}

func (p *continuumProvider) Configure(
	ctx context.Context, req tf_provider.ConfigureRequest, resp *tf_provider.ConfigureResponse,
) {
	// Inject the key into the config rather than the HCL so it never lands in
	// the config file the test framework writes to disk.
	var attrs map[string]tftypes.Value
	if err := req.Config.Raw.As(&attrs); err != nil {
		resp.Diagnostics.AddError("continuum test provider", err.Error())
		return
	}
	attrs["apikey"] = tftypes.NewValue(tftypes.String, p.apiKey)
	req.Config.Raw = tftypes.NewValue(req.Config.Raw.Type(), attrs)
	p.provider.Configure(ctx, req, resp)
}

func continuumProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	p := &continuumProvider{
		provider: New("test")().(*provider),
		apiKey:   os.Getenv(cockroachContinuumAPIKey),
	}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"cockroach": providerserver.NewProtocol6WithError(p),
	}
}

func TestUserAgent(t *testing.T) {
	require.Equal(t,
		"terraform-provider-cockroach/1.22.0 terraform/1.9.5",
		userAgent("1.22.0", "1.9.5"),
	)
	require.Equal(t,
		"terraform-provider-cockroach/dev",
		userAgent("dev", ""),
	)
}

func TestClientConfigurationSetsCcClientHeader(t *testing.T) {
	cfg := getClientConfiguration("key", "", "1.22.0", "1.9.5")
	require.Equal(t, "terraform", cfg.DefaultHeader["Cc-Client"])
}

// GenerateRandomString returns n random lowercase letters. While a test is
// recording or replaying, the letters come from the recording's seed.
func GenerateRandomString(n int) string {
	if r := currentRecorder(); r != nil {
		return r.randomString(n)
	}
	letters := "abcdefghijklmnopqrstuvwxyz"
	letterRunes := []rune(letters)
	b := make([]rune, n)
	for i := range b {
		randNum, _ := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(letterRunes))))
		b[i] = letterRunes[randNum.Int64()]
	}
	return string(b)
}
