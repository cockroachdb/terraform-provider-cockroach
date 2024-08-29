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
	cryptorand "crypto/rand"
	"math/big"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v2/pkg/client"
	tf_provider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var testAccProvider tf_provider.Provider
var cl *client.Client

func init() {
	apikey := os.Getenv(CockroachAPIKey)
	cfg := client.NewConfiguration(apikey)
	if server := os.Getenv(APIServerURLKey); server != "" {
		cfg.ServerURL = server
	}
	cfg.UserAgent = UserAgent
	cl = client.NewClient(cfg)
	testAccProvider = New("test")()
}

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"cockroach": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		t.Fatalf("%s must be set for acceptance testing", CockroachAPIKey)
	}
}

func GenerateRandomString(n int) string {
	letters := "abcdefghijklmnopqrstuvwxyz"
	letterRunes := []rune(letters)
	b := make([]rune, n)
	for i := range b {
		randNum, _ := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(letterRunes))))
		b[i] = letterRunes[randNum.Int64()]
	}
	return string(b)
}
