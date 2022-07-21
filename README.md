# Terraform provider for CockroachDB Cloud

`terraform-provider-cockroach` is the [Terraform provider](https://learn.hashicorp.com/collections/terraform/providers) for the CockroachDB Cloud API [[reference](https://www.cockroachlabs.com/docs/api/cloud/v1), [getting started](https://www.cockroachlabs.com/docs/cockroachcloud/cloud-api)].

- [More information about CockroachDB](https://www.cockroachlabs.com/)
- [More information about Terraform](https://terraform.io) 

**Note: This is a preview release, suitable only for experimental use.**

# Get Started

## Prerequisites

Before you use `terraform-provider-cockroach` you must [install Terraform](https://learn.hashicorp.com/tutorials/terraform/install-cli) and [`git`](https://git-scm.com/downloads) on your local machine.

## Installation

1. In a terminal clone the `terraform-provider-cockroach` repository:

    ~~~ shell
    git clone https://github.com/cockroachdb/terraform-provider-cockroach.git
    ~~~

1. Navigate to the `terraform-provider-cockroach` directory.

    ~~~ shell
    cd terraform-proivder-cockroach
    ~~~

1. Build the binary and copy it to your path.

    ~~~ shell
    make install
    ~~~

## Run the Serverless Example

1. In a terminal go to the `examples/workflows/cockroach_serverless_cluster` directory.

    ~~~ shell 
    cd examples/workflows/cockroach_serverless_cluster
    ~~~

1. The provider requires an API key set in an environment variable named `COCKROACH_API_KEY`. Copy the [API key](https://www.cockroachlabs.com/docs/cockroachcloud/console-access-management#api-access) from the CockroachDB Cloud console and create the `COCKROACH_API_KEY` environment variable.

    ~~~ shell
    export COCKROACH_API_KEY=<YOUR_API_KEY>
    ~~~
    
    Where `<your API key>` is the API key you copied from the CockroachDB Cloud Console.

1. In a text editor create a new file `terraform.tfvars` in `serverless` with the following settings.

    ~~~
    cluster_name = "<cluster name>"
    sql_user_name = "<SQL user name>"
    sql_user_password = "<SQL user password>"
    ~~~ 

    Where:
        - `<cluster name>` is the name of the cluster you want to create.
        - `<SQL user name>` is the name of the SQL user you want to create.
        - `<SQL user password>` is the password for the SQL user you want to create.

1. Initialize the provider.

    ~~~ shell
    terraform init
    ~~~
    
    This reads the `main.tf` configuration file, which contains the information on how the provider will create the Serverless cluster. The `terraform.tfvars` file sets the cluster name, SQL user name, and SQL user password.

1. Create the Terraform plan. This shows the actions the provider will take, but won't perform them.

    ~~~ shell
    terraform plan
    ~~~

1. Create the cluster.

    ~~~ shell
    terraform apply
    ~~~
    
    Enter `yes` when prompted to apply the plan and create the cluster.

1. (optional) Delete the cluster when you are done.

    ~~~ shell
    terraform destroy
    ~~~

    Enter `yes` when prompted to delete the cluster.
    
## Run the Dedicated Example

1. In a terminal go to the `examples/workflows/cockroach_dedicated_cluster` directory.

    ~~~ shell 
    cd examples/workflows/cockroach_dedicated_cluster
    ~~~

1. The provider requires an API key set in an environment variable named `COCKROACH_API_KEY`. Copy the [API key](https://www.cockroachlabs.com/docs/cockroachcloud/console-access-management#api-access) from the CockroachDB Cloud console and create the `COCKROACH_API_KEY` environment variable.

    ~~~ shell
    export COCKROACH_API_KEY=<YOUR_API_KEY>
    ~~~
    
    Where `<your API key>` is the API key you copied from the CockroachDB Cloud Console.

1. In a text editor create a new file `terraform.tfvars` with the following settings.

    ~~~
    cluster_name = "<cluster name>"
    sql_user_name = "<SQL user name>"
    sql_user_password = "<SQL user password>"
    cloud_provider = "<cloud provider>"
    cloud_provider_region = ["<cloud provider region>"]
    cluster_nodes = <number of nodes>
    storage_gib = <storage in GiB>
    machine_type = "<cloud provider machine type>"
    allow_list_name = "<allow list name>"
    cidr_ip = "<allow list CIDR IP>"
    cidr_mask = <allow list CIDR mask>
    ~~~ 

    Where:
        - `<cluster name>` is the name of the cluster you want to create.
        - `<SQL user name>` is the name of the SQL user you want to create.
        - `<SQL user password>` is the password for the SQL user you want to create.
        - `<cloud provider>` is the cloud infrastructure provider. Possible values are `GCP` or `AWS`.
        - `<cloud provider region>` is the region code or codes for the cloud infrastructure provider. For multi-region clusters, separate each region with a comma.
        - `<number of nodes>` is the number of nodes in each region. Cockroach Labs recommends at least 3 nodes per region, and the same number of nodes in each region for multi-region clusters.
        - `<storage in GiB>` is the amount of storage specified in GiB.
        - `<cloud provider machine type>` is the machine type for the cloud infrastructure provider.
        - `<allow list name>` is the name for the IP allow list. Use a descriptive name to identify the IP allow list.
        - `<allow list CIDR IP>` is the Classless Inter-Domain Routing (CIDR) IP address base.
        - `<allow list CIDR mask>` is the CIDR mask.

1. Initialize the provider.

    ~~~ shell
    terraform init
    ~~~
    
    This reads the `main.tf` configuration file, which contains the information on how the provider will create the Serverless cluster. The `terraform.tfvars` file sets the cluster name, SQL user name, and SQL user password.

1. Create the Terraform plan. This shows the actions the provider will take, but won't perform them.

    ~~~ shell
    terraform plan
    ~~~

1. Create the cluster.

    ~~~ shell
    terraform apply
    ~~~
    
    Enter `yes` when prompted to apply the plan and create the cluster.

1. (optional) Delete the cluster when you are done.

    ~~~ shell
    terraform destroy
    ~~~

    Enter `yes` when prompted to delete the cluster.
    
