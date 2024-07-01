---
page_title: "redpanda_serverless_cluster Resource - terraform-provider-redpanda"
subcategory: ""
description: |-
  
---

# redpanda_serverless_cluster (Resource)



Enables the provisioning and management of Redpanda serverless clusters on AWS. A serverless cluster must always have a resource group.

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `name` (String) Name of the serverless cluster
- `resource_group_id` (String) The ID of the Resource Group in which to create the serverless cluster

### Optional

- `serverless_region` (String) Redpanda specific region of the serverless cluster

### Read-Only

- `cluster_api_url` (String) The URL of the dataplane API for the serverless cluster
- `id` (String) The ID of the serverless cluster

## Usage

### On AWS

```terraform
provider "redpanda" {
}
resource "redpanda_resource_group" "test" {
  name = var.resource_group_name
}

resource "redpanda_serverless_cluster" "test" {
  name              = var.cluster_name
  resource_group_id = redpanda_resource_group.test.id
  serverless_region = var.region
}

variable "resource_group_name" {
  default = "testgroup"
}

variable "cluster_name" {
  default = "testname"
}

variable "region" {
  default = "pro-us-east-1"
}
```

## Limitations

We are not currently able to support the provisioning of serverless clusters on GCP.

### Example Usage to create a serverless cluster

```terraform
provider "redpanda" {
}
resource "redpanda_resource_group" "test" {
  name = var.resource_group_name
}

resource "redpanda_serverless_cluster" "test" {
  name              = var.cluster_name
  resource_group_id = redpanda_resource_group.test.id
  serverless_region = var.region
}

variable "resource_group_name" {
  default = "testgroup"
}

variable "cluster_name" {
  default = "testname"
}

variable "region" {
  default = "pro-us-east-1"
}
```

## Import

```shell
terraform import resource.redpanda_serverless_cluster.example serverlessClusterId
```
