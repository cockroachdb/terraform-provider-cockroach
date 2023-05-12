# Developing `terraform-provider-cockroach`

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

## Environment Variables

To override the hostname of the cockroach cloud api server used by the
terraform provider

~~~shell
# no trailing slash!
export COCKROACH_SERVER=https://qaserver.com
~~~
