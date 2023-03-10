/*
 Copyright 2022 The Cockroach Authors

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
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestIntegrationClientCACertResource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-cmek-%s", GenerateRandomString(4))
	clusterId := "cluster-id"
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
		CloudProvider: "AWS",
		State:         "CREATED",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				MachineType: "m5.xlarge",
				StorageGib:  35,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-central-1",
				NodeCount: 3,
			},
		},
	}

	cert1InfoPending := client.NewClientCACertInfo()
	cert1InfoPending.SetX509PemCert(cert1)
	cert1InfoPending.SetStatus(client.CLIENTCACERTSTATUS_PENDING)
	cert1InfoReady := client.NewClientCACertInfo()
	cert1InfoReady.SetX509PemCert(cert1)
	cert1InfoReady.SetStatus(client.CLIENTCACERTSTATUS_IS_SET)

	cert2InfoPending := client.NewClientCACertInfo()
	cert2InfoPending.SetX509PemCert(cert2)
	cert2InfoPending.SetStatus(client.CLIENTCACERTSTATUS_PENDING)
	cert2InfoReady := client.NewClientCACertInfo()
	cert2InfoReady.SetX509PemCert(cert2)
	cert2InfoReady.SetStatus(client.CLIENTCACERTSTATUS_IS_SET)

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
		Return(cert2InfoReady, nil, nil) // CERT Read()

	s.EXPECT().DeleteClientCACert(gomock.Any(), clusterId) // CERT Delete()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterId)      // CLUSTER Delete()

	testClientCACertResource(t, clusterName, clusterId, cert1, cert2, true)
}

func testClientCACertResource(t *testing.T, clusterName string, clusterId string, cert1 string, cert2 string, useMock bool) {
	clusterResourceName := "cockroach_cluster.test"
	cliCACertResourceName := "cockroach_client_ca_cert.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestClientCACertResourceCreateConfig(clusterName, cert1),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(clusterResourceName, "name", clusterName),
					resource.TestCheckResourceAttr(cliCACertResourceName, "id", clusterId),
					resource.TestCheckResourceAttr(cliCACertResourceName, "x509_pem_cert", cert1),
					resource.TestCheckResourceAttr(cliCACertResourceName, "status", string(client.CLIENTCACERTSTATUS_IS_SET)),
				),
			},
			{
				Config: getTestClientCACertResourceUpdateConfig(clusterName, cert2),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(clusterResourceName, "name", clusterName),
					resource.TestCheckResourceAttr(cliCACertResourceName, "id", clusterId),
					resource.TestCheckResourceAttr(cliCACertResourceName, "x509_pem_cert", cert2),
					resource.TestCheckResourceAttr(cliCACertResourceName, "status", string(client.CLIENTCACERTSTATUS_IS_SET)),
				),
			},
		},
	})
}

func getTestClientCACertResourceCreateConfig(name string, cert string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
	name           = "%s"
	cloud_provider = "AWS"
	dedicated = {
		storage_gib = 35
		machine_type = "m5.xlarge"
	}
	regions = [{
		name = "us-central-1"
		node_count: 3
	}]
}

resource "cockroach_client_ca_cert" "test" {
	id = cockroach_cluster.test.id
	x509_pem_cert = "%s"
}
`, name, cert)
}

func getTestClientCACertResourceUpdateConfig(name string, cert string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
	name           = "%s"
	cloud_provider = "AWS"
	dedicated = {
		storage_gib = 35
		machine_type = "m5.xlarge"
	}
	regions = [{
		name = "us-central-1"
		node_count: 3
	}]
}

resource "cockroach_client_ca_cert" "test" {
	id = cockroach_cluster.test.id
	x509_pem_cert = "%s"
}
`, name, cert)
}
