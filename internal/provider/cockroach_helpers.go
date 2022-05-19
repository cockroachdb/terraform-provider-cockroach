package provider

import (
	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func loadClusterToTerraformState(clusterObj *client.Cluster) *CockroachCluster {
	var rgs []Region
	for _, x := range clusterObj.Regions {
		rg := Region{
			Name:      types.String{Value: x.Name},
			SqlDns:    types.String{Value: x.SqlDns},
			UiDns:     types.String{Value: x.UiDns},
			NodeCount: types.Int64{Value: int64(x.NodeCount)},
		}
		rgs = append(rgs, rg)
	}

	var state = CockroachCluster{
		ID:               types.String{Value: clusterObj.Id},
		Name:             types.String{Value: clusterObj.Name},
		CockroachVersion: types.String{Value: clusterObj.CockroachVersion},
		Plan:             types.String{Value: string(clusterObj.Plan)},
		CloudProvider:    ApiCloudProvider(clusterObj.CloudProvider),
		AccountId:        types.String{Value: *clusterObj.AccountId},
		State:            types.String{Value: string(clusterObj.State)},
		CreatorId:        types.String{Value: clusterObj.CreatorId},
		OperationStatus:  types.String{Value: string(clusterObj.OperationStatus)},
		Config:           &ClusterConfig{},
		Regions:          rgs,
	}

	if clusterObj.Config.Serverless != nil {
		state.Config.Serverless = &ServerlessClusterConfig{
			SpendLimit: types.Int64{Value: int64(clusterObj.Config.Serverless.SpendLimit)},
			RoutingId:  types.String{Value: clusterObj.Config.Serverless.RoutingId},
		}
	} else if clusterObj.Config.Dedicated != nil {
		state.Config.Dedicated = &DedicatedHardwareConfig{
			MachineType:    types.String{Value: clusterObj.Config.Dedicated.MachineType},
			NumVirtualCpus: types.Int64{Value: int64(clusterObj.Config.Dedicated.NumVirtualCpus)},
			StorageGib:     types.Int64{Value: int64(clusterObj.Config.Dedicated.StorageGib)},
			MemoryGib:      types.Float64{Value: float64(clusterObj.Config.Dedicated.MemoryGib)},
			DiskIops:       types.Int64{Value: int64(clusterObj.Config.Dedicated.DiskIops)},
		}
	}

	return &state
}
