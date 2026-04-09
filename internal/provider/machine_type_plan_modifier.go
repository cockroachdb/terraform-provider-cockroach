package provider

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v7/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// azureVCPUCountRE extracts the vCPU count from Azure machine types.
// See https://learn.microsoft.com/en-us/azure/virtual-machines/vm-naming-conventions
// Format: Standard_[Family][Subfamily]*[vCPUs]..., e.g. "Standard_D4s_v3" → 4.
var azureVCPUCountRE = regexp.MustCompile(`^Standard_[A-Z]+(\d+)`)

// awsSizeToVCPUs maps AWS instance size suffixes to vCPU counts.
var awsSizeToVCPUs = map[string]int{
	"medium":   1,
	"large":    2,
	"xlarge":   4,
	"2xlarge":  8,
	"4xlarge":  16,
	"8xlarge":  32,
	"9xlarge":  36,
	"12xlarge": 48,
	"16xlarge": 64,
	"18xlarge": 72,
	"24xlarge": 96,
}

// SuppressMachineTypeDrift returns a plan modifier for the machine_type
// attribute. When the user does not set machine_type in config (using
// num_virtual_cpus instead), it acts as UseStateForUnknown to prevent
// "(known after apply)" noise.
//
// Drift suppression for instance family migrations is handled in
// loadClusterToTerraformState, which preserves the previous
// state value when the vCPU count is unchanged. This allows explicit user
// changes to always reach the server.
func SuppressMachineTypeDrift() planmodifier.String {
	return machineTypeDriftSuppressor{}
}

type machineTypeDriftSuppressor struct{}

func (m machineTypeDriftSuppressor) Description(_ context.Context) string {
	return "Uses state value for machine_type when not set in config (num_virtual_cpus users)."
}

func (m machineTypeDriftSuppressor) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m machineTypeDriftSuppressor) PlanModifyString(
	_ context.Context,
	req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	// No state (creation): nothing to do.
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}

	// Config is null (user uses num_virtual_cpus, not machine_type).
	// Use state value to prevent "(known after apply)" noise.
	if req.ConfigValue.IsNull() {
		resp.PlanValue = req.StateValue
		return
	}
}

// extractVCPUCount extracts the vCPU count from a machine type string using
// the cloud provider to determine the format. Returns (0, false) if the
// machine type cannot be parsed.
func extractVCPUCount(cloudProvider client.CloudProviderType, machineType string) (int, bool) {
	switch cloudProvider {
	case client.CLOUDPROVIDERTYPE_AWS:
		// See https://docs.aws.amazon.com/ec2/latest/instancetypes/instance-type-names.html
		// Format: [family][generation][attributes].[size], e.g. "m6i.xlarge" → 4.
		parts := strings.SplitN(machineType, ".", 2)
		if len(parts) != 2 {
			return 0, false
		}
		vcpus, ok := awsSizeToVCPUs[parts[1]]
		return vcpus, ok

	case client.CLOUDPROVIDERTYPE_GCP:
		// See https://docs.cloud.google.com/compute/docs/machine-resource
		// Format: [family]-[type]-[vCPUs], e.g. "n2-standard-4" → 4.
		parts := strings.Split(machineType, "-")
		if len(parts) < 3 {
			return 0, false
		}
		n, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil || n <= 0 {
			return 0, false
		}
		return n, true

	case client.CLOUDPROVIDERTYPE_AZURE:
		matches := azureVCPUCountRE.FindStringSubmatch(machineType)
		if len(matches) != 2 {
			return 0, false
		}
		n, err := strconv.Atoi(matches[1])
		if err != nil || n <= 0 {
			return 0, false
		}
		return n, true

	default:
		return 0, false
	}
}

// Compile-time check that machineTypeDriftSuppressor implements the interface.
var _ planmodifier.String = machineTypeDriftSuppressor{}
