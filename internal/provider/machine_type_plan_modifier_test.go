package provider

import (
	"context"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v7/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestExtractVCPUCount(t *testing.T) {
	tests := []struct {
		name          string
		cloudProvider client.CloudProviderType
		machineType   string
		wantVCPUs     int
		wantOk        bool
	}{
		// AWS
		{name: "aws m6i.medium", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.medium", wantVCPUs: 1, wantOk: true},
		{name: "aws m6i.large", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.large", wantVCPUs: 2, wantOk: true},
		{name: "aws m6i.xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.xlarge", wantVCPUs: 4, wantOk: true},
		{name: "aws m7g.xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m7g.xlarge", wantVCPUs: 4, wantOk: true},
		{name: "aws m6i.2xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.2xlarge", wantVCPUs: 8, wantOk: true},
		{name: "aws m7g.2xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m7g.2xlarge", wantVCPUs: 8, wantOk: true},
		{name: "aws m6i.4xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.4xlarge", wantVCPUs: 16, wantOk: true},
		{name: "aws m6i.8xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.8xlarge", wantVCPUs: 32, wantOk: true},
		{name: "aws m6i.12xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.12xlarge", wantVCPUs: 48, wantOk: true},
		{name: "aws m6i.16xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.16xlarge", wantVCPUs: 64, wantOk: true},
		{name: "aws m6i.24xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.24xlarge", wantVCPUs: 96, wantOk: true},
		{name: "aws c5.xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "c5.xlarge", wantVCPUs: 4, wantOk: true},
		{name: "aws r6i.xlarge", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "r6i.xlarge", wantVCPUs: 4, wantOk: true},
		{name: "aws m6i.metal", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.metal", wantVCPUs: 0, wantOk: false},
		{name: "aws unknown size", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "m6i.foo", wantVCPUs: 0, wantOk: false},

		// GCP
		{name: "gcp n2-standard-4", cloudProvider: client.CLOUDPROVIDERTYPE_GCP, machineType: "n2-standard-4", wantVCPUs: 4, wantOk: true},
		{name: "gcp n2-standard-8", cloudProvider: client.CLOUDPROVIDERTYPE_GCP, machineType: "n2-standard-8", wantVCPUs: 8, wantOk: true},
		{name: "gcp n2-standard-16", cloudProvider: client.CLOUDPROVIDERTYPE_GCP, machineType: "n2-standard-16", wantVCPUs: 16, wantOk: true},
		{name: "gcp c3-standard-4", cloudProvider: client.CLOUDPROVIDERTYPE_GCP, machineType: "c3-standard-4", wantVCPUs: 4, wantOk: true},
		{name: "gcp n2-highmem-8", cloudProvider: client.CLOUDPROVIDERTYPE_GCP, machineType: "n2-highmem-8", wantVCPUs: 8, wantOk: true},

		// Azure
		{name: "azure Standard_D4s_v3", cloudProvider: client.CLOUDPROVIDERTYPE_AZURE, machineType: "Standard_D4s_v3", wantVCPUs: 4, wantOk: true},
		{name: "azure Standard_D8s_v3", cloudProvider: client.CLOUDPROVIDERTYPE_AZURE, machineType: "Standard_D8s_v3", wantVCPUs: 8, wantOk: true},
		{name: "azure Standard_D16s_v3", cloudProvider: client.CLOUDPROVIDERTYPE_AZURE, machineType: "Standard_D16s_v3", wantVCPUs: 16, wantOk: true},
		{name: "azure Standard_E4s_v4", cloudProvider: client.CLOUDPROVIDERTYPE_AZURE, machineType: "Standard_E4s_v4", wantVCPUs: 4, wantOk: true},

		// Unrecognized
		{name: "empty string", cloudProvider: client.CLOUDPROVIDERTYPE_AWS, machineType: "", wantVCPUs: 0, wantOk: false},
		{name: "unknown provider", cloudProvider: "UNKNOWN", machineType: "m6i.xlarge", wantVCPUs: 0, wantOk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vcpus, ok := extractVCPUCount(tt.cloudProvider, tt.machineType)
			require.Equal(t, tt.wantOk, ok, "ok mismatch for %s", tt.machineType)
			if ok {
				require.Equal(t, tt.wantVCPUs, vcpus, "vcpu mismatch for %s", tt.machineType)
			}
		})
	}
}

func TestSuppressMachineTypeDrift(t *testing.T) {
	ctx := context.Background()
	modifier := SuppressMachineTypeDrift()

	tests := []struct {
		name         string
		configValue  types.String
		stateValue   types.String
		planValue    types.String
		expectedPlan types.String
		description  string
	}{
		{
			name:         "create - null state",
			configValue:  types.StringValue("m6i.xlarge"),
			stateValue:   types.StringNull(),
			planValue:    types.StringValue("m6i.xlarge"),
			expectedPlan: types.StringValue("m6i.xlarge"),
			description:  "On create, state is null, plan should pass through unchanged",
		},
		{
			name:         "no drift - config equals state",
			configValue:  types.StringValue("m6i.xlarge"),
			stateValue:   types.StringValue("m6i.xlarge"),
			planValue:    types.StringValue("m6i.xlarge"),
			expectedPlan: types.StringValue("m6i.xlarge"),
			description:  "No drift, plan should remain unchanged",
		},
		{
			name:         "config differs from state - diff allowed",
			configValue:  types.StringValue("m6i.xlarge"),
			stateValue:   types.StringValue("m7g.xlarge"),
			planValue:    types.StringValue("m6i.xlarge"),
			expectedPlan: types.StringValue("m6i.xlarge"),
			description:  "Config differs from state, diff passes through to server",
		},
		{
			name:         "different vcpu - diff allowed",
			configValue:  types.StringValue("m6i.xlarge"),
			stateValue:   types.StringValue("m7g.2xlarge"),
			planValue:    types.StringValue("m6i.xlarge"),
			expectedPlan: types.StringValue("m6i.xlarge"),
			description:  "Different vCPUs (4 vs 8), diff should be allowed",
		},
		{
			name:         "null config - uses state value",
			configValue:  types.StringNull(),
			stateValue:   types.StringValue("m7g.xlarge"),
			planValue:    types.StringUnknown(),
			expectedPlan: types.StringValue("m7g.xlarge"),
			description:  "User uses num_virtual_cpus, machine_type should use state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := planmodifier.StringRequest{
				ConfigValue: tt.configValue,
				StateValue:  tt.stateValue,
				PlanValue:   tt.planValue,
			}
			resp := &planmodifier.StringResponse{
				PlanValue: tt.planValue,
			}

			modifier.PlanModifyString(ctx, req, resp)

			require.Equal(t, tt.expectedPlan, resp.PlanValue, tt.description)
			require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics error")
		})
	}
}
