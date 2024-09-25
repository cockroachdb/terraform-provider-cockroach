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
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v4/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccClientCACertResource attempts to create, check, and destroy
// a real cluster and client CA certs. It will be skipped if TF_ACC isn't set.
func TestAccClientCACertResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-clcacert-%s", tfTestPrefix, GenerateRandomString(3))

	// test cert with CN=test1
	cert1 := "-----BEGIN CERTIFICATE-----\nMIIDgTCCAmigAwIBAgIBADANBgkqhkiG9w0BAQ0FADBaMQswCQYDVQQGEwJ1czEL\nMAkGA1UECAwCV0ExDjAMBgNVBAoMBXRlc3QxMQ4wDAYDVQQDDAV0ZXN0MTEOMAwG\nA1UEBwwFdGVzdDExDjAMBgNVBAsMBXRlc3QxMB4XDTIzMDMyMTE5MDczN1oXDTI2\nMDMyMDE5MDczN1owWjELMAkGA1UEBhMCdXMxCzAJBgNVBAgMAldBMQ4wDAYDVQQK\nDAV0ZXN0MTEOMAwGA1UEAwwFdGVzdDExDjAMBgNVBAcMBXRlc3QxMQ4wDAYDVQQL\nDAV0ZXN0MTCCASMwDQYJKoZIhvcNAQEBBQADggEQADCCAQsCggECAL8DDMgY6Vck\nL1VfzXRXsGVYIdq/NQdVLUAUB/7WCd6/lVGvCMn/iR1uYUSDhTvW2CB4NqbFBE9Y\nBwS5OZMqficezfR8xxRDsIIqjSlpWzE0j7uROyCFxXpt9PyIgTVX2Hj8xov8Y7aT\nS5yddCURxjTC5/Fllwizn7PcJAqOcpzlqddHUXvcGKrEljyAf6fbpcCug4tR3Fq3\ngf5rfwm1YR8w+GyFRiWy3B27oIftEEZ4GaN/J9+d5MZGS00PemilcQ3HyD20k7VU\nbTylkYoDbTrt3ivK1SllkmOHtQ8eBZCF3S9kpctnF9ohy6k2F16wdLQ/XcGhMG5d\nJ9eRWAXfy2EpAgMBAAGjUDBOMB0GA1UdDgQWBBR+mcFGBlCHOQe3H49JSdZMrqqS\nnDAfBgNVHSMEGDAWgBR+mcFGBlCHOQe3H49JSdZMrqqSnDAMBgNVHRMEBTADAQH/\nMA0GCSqGSIb3DQEBDQUAA4IBAgAZwBCWrvwxTnJ/3LtE8yG5tfhjoXZg3q19ZrNh\nza0fElnvxONgNtLHjjNCWaf3er9EGW/Ge2NWuMGHjTwRwG9VzFETcnDrZVLf6uF7\noriyzsno0Ob5VZkgeyFw23l/nEzGYtVM549Fz65trzyASOtqi1AH95xTd6fh3goq\nd9ivSkP1FzGr4fwz8k9LQhE8YASgH6mZ5dO3ZNQjYzGsU7T5MIK/TYoyS4DDKQMM\nGbhUuJXSoTsHrs9Jma6mGoBQaxJet6RfTk1+fDnCl9OdlS66Ez3V7KsLJ4Lzzi8S\nJh5teJQymchzWt4Imn7VSHqedIF9psNaJ/ucHVaJAzkKBJGTzA==\n-----END CERTIFICATE-----"
	// test cert with CN=test2
	cert2 := "-----BEGIN CERTIFICATE-----\nMIIDgTCCAmigAwIBAgIBADANBgkqhkiG9w0BAQ0FADBaMQswCQYDVQQGEwJ1czEL\nMAkGA1UECAwCV0ExDjAMBgNVBAoMBXRlc3QyMQ4wDAYDVQQDDAV0ZXN0MjEOMAwG\nA1UEBwwFdGVzdDIxDjAMBgNVBAsMBXRlc3QyMB4XDTIzMDMyMTE5MTI1MVoXDTI2\nMDMyMDE5MTI1MVowWjELMAkGA1UEBhMCdXMxCzAJBgNVBAgMAldBMQ4wDAYDVQQK\nDAV0ZXN0MjEOMAwGA1UEAwwFdGVzdDIxDjAMBgNVBAcMBXRlc3QyMQ4wDAYDVQQL\nDAV0ZXN0MjCCASMwDQYJKoZIhvcNAQEBBQADggEQADCCAQsCggECAL2hDCIH8t0e\nIEGZlA44ekUu94EAFREnCu6UFAMvxCJVa3rfpNFIKxBRQTf0NL2K0hp4imx8PbyT\n761U1NO3eFzy/CzXzf3RazdxU619wf/5IYg0fFyD1JfGd+tsbQMILkM14nIFcoVb\ni7bxUVV4w7U3YIUCi811j/tabjTRmzFs+v2TiKcRRjNbIHCkBNAC+wt+XFhbL6oa\nAqlpcMw9geris/ovD4kKaTLgAa0pxXiUQcYwBEuZoKzNTf1Un8A534HoKsU7lCND\n/sz5haiHd4znzQ1pyY1GDoNT97XM92y9c2o4niNSedlkp7vlwwLseBz232qBs9mm\nWdON2UYP74ohAgMBAAGjUDBOMB0GA1UdDgQWBBQnjmzTCRtNbO3RkA1Iwlh/xzQW\nATAfBgNVHSMEGDAWgBQnjmzTCRtNbO3RkA1Iwlh/xzQWATAMBgNVHRMEBTADAQH/\nMA0GCSqGSIb3DQEBDQUAA4IBAgApDx+zeTO+oLKus2n3v8blrwpmgms6tAvsh2kX\ndGTdJForrv8hFMfkaVfSls+QwP6aMaDntdhM+cl+0BbDrxr4UYp2q7c3fESSnd5f\nx+J4QReyMa2mt5nUf3iWdM2VhM5Ie5zcsWysnakzHs6Vo2anHjeZvNZxcwopzVaa\nxvaBTduhzzqnzQZtr3D2qhtsOOKf3m5NprLnuwZ8gWDjpXnaR38R/w/KMtPKQRum\n/hRf5AJfC5UrbMbnhMmEuO9ODnct/ejabbU/fTNbo2jeAXZusGYdDxNXuhCloSwJ\nEOogpErNxiaLV5JVrRNtb2DPkdJKkaEUbjr63f+T3BcH7H0pLw==\n-----END CERTIFICATE-----"

	testClientCACertResource(t, clusterName, cert1, cert2, false)
}

func TestIntegrationClientCACertResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-clcacert-%s", tfTestPrefix, GenerateRandomString(3))
	clusterId := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	cert1 := "test-cert-1"
	cert2 := "test-cert-2"

	cluster := &client.Cluster{
		Id:            clusterId,
		Name:          clusterName,
		CloudProvider: "GCP",
		State:         "CREATED",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				MachineType:    "m5.xlarge",
				NumVirtualCpus: 4,
				StorageGib:     35,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-west2",
				NodeCount: 1,
			},
		},
	}

	cert1InfoPending := client.NewClientCACertInfo()
	cert1InfoPending.SetStatus(client.CLIENTCACERTSTATUS_PENDING)
	cert1InfoReady := client.NewClientCACertInfo()
	cert1InfoReady.SetX509PemCert(cert1)
	cert1InfoReady.SetStatus(client.CLIENTCACERTSTATUS_IS_SET)

	cert2InfoPending := client.NewClientCACertInfo()
	cert2InfoPending.SetStatus(client.CLIENTCACERTSTATUS_PENDING)
	cert2InfoReady := client.NewClientCACertInfo()
	cert2InfoReady.SetX509PemCert(cert2)
	cert2InfoReady.SetStatus(client.CLIENTCACERTSTATUS_IS_SET)

	certInfoEmptyPending := client.NewClientCACertInfo()
	certInfoEmptyPending.SetStatus(client.CLIENTCACERTSTATUS_PENDING)
	certInfoEmptyReady := client.NewClientCACertInfo()
	certInfoEmptyReady.SetStatus(client.CLIENTCACERTSTATUS_IS_SET)

	httpOkResponse := &http.Response{Status: http.StatusText(http.StatusOK)}

	// CREATE
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil) // CLUSTER Create()
	s.EXPECT().GetCluster(gomock.Any(), clusterId).
		Return(cluster, httpOkResponse, nil) // CLUSTER Create()/waitReady()

	s.EXPECT().GetCluster(gomock.Any(), clusterId).
		Return(cluster, httpOkResponse, nil) // CERT Create(): dedicated check
	s.EXPECT().SetClientCACert(gomock.Any(), clusterId, gomock.Any()).
		Return(cert1InfoPending, nil, nil) // CERT Create(): set cert console call, initially return PENDING to trigger retry
	s.EXPECT().GetClientCACert(gomock.Any(), clusterId).
		Return(cert1InfoPending, nil, nil) // CERT Create()/waitReady(): return PENDING, continue polling
	s.EXPECT().GetClientCACert(gomock.Any(), clusterId).
		Return(cert1InfoReady, nil, nil) // CERT Create()/waitReady(): now return IS_SET

	s.EXPECT().GetCluster(gomock.Any(), clusterId).
		Return(cluster, httpOkResponse, nil) // CLUSTER Read()
	s.EXPECT().GetClientCACert(gomock.Any(), clusterId).
		Return(cert1InfoReady, nil, nil) // CERT Read()
	s.EXPECT().GetCluster(gomock.Any(), clusterId).
		Return(cluster, httpOkResponse, nil) // CLUSTER Read()
	s.EXPECT().GetClientCACert(gomock.Any(), clusterId).
		Return(cert1InfoReady, nil, nil) // CERT Read()

	// UPDATE
	s.EXPECT().UpdateClientCACert(gomock.Any(), clusterId, gomock.Any()).
		Return(cert2InfoPending, nil, nil) // CERT Update()
	s.EXPECT().GetClientCACert(gomock.Any(), clusterId).
		Return(cert2InfoPending, nil, nil) // CERT Update()/waitReady(): return PENDING, continue polling
	s.EXPECT().GetClientCACert(gomock.Any(), clusterId).
		Return(cert2InfoReady, nil, nil) // CERT Update()/waitReady(): now return IS_SET

	// TF TEST FRAMEWORK: TEAR DOWN
	s.EXPECT().GetCluster(gomock.Any(), clusterId).
		Return(cluster, httpOkResponse, nil) // CLUSTER Read()
	s.EXPECT().GetClientCACert(gomock.Any(), clusterId).
		Return(cert2InfoReady, nil, nil).Times(2) // CERT Read()

	s.EXPECT().DeleteClientCACert(gomock.Any(), clusterId).
		Return(certInfoEmptyPending, nil, nil) // CERT Delete()
	s.EXPECT().GetClientCACert(gomock.Any(), clusterId).
		Return(certInfoEmptyPending, nil, nil) // CERT Delete()/waitReady(): return PENDING, continue polling
	s.EXPECT().GetClientCACert(gomock.Any(), clusterId).
		Return(certInfoEmptyReady, nil, nil) // CERT Delete()/waitReady(): now return IS_SET

	s.EXPECT().DeleteCluster(gomock.Any(), clusterId) // CLUSTER Delete()

	testClientCACertResource(t, clusterName, cert1, cert2, true)
}

func testClientCACertResource(t *testing.T, clusterName string, cert1 string, cert2 string, useMock bool) {
	clusterResourceName := "cockroach_cluster.test"
	cliCACertResourceName := "cockroach_client_ca_cert.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestClientCACertResourceConfig(clusterName, cert1),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(clusterResourceName, "name", clusterName),
					resource.TestCheckResourceAttr(cliCACertResourceName, "x509_pem_cert", cert1),
					resource.TestCheckResourceAttr(cliCACertResourceName, "status", string(client.CLIENTCACERTSTATUS_IS_SET)),
				),
			},
			{
				Config: getTestClientCACertResourceConfig(clusterName, cert2),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(clusterResourceName, "name", clusterName),
					resource.TestCheckResourceAttr(cliCACertResourceName, "x509_pem_cert", cert2),
					resource.TestCheckResourceAttr(cliCACertResourceName, "status", string(client.CLIENTCACERTSTATUS_IS_SET)),
				),
			},
			{
				ImportState:       true,
				ImportStateVerify: true,
				ResourceName:      cliCACertResourceName,
			},
		},
	})
}

func getTestClientCACertResourceConfig(name string, cert string) string {
	escapedCert := strings.ReplaceAll(cert, "\n", "\\n")

	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
	name           = "%s"
	cloud_provider = "GCP"
	dedicated = {
		storage_gib = 35
		num_virtual_cpus = 4
	}
	regions = [{
		name = "us-west2"
		node_count: 1
	}]
}

resource "cockroach_client_ca_cert" "test" {
	id = cockroach_cluster.test.id
	x509_pem_cert = "%s"
}
`, name, escapedCert)
}
