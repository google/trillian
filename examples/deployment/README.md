Deploying Trillian
==================

Want to deploy/use the Trillian General Transparency project in the cloud?  Here are some common ways of getting off the ground with Docker.

## Setup

**Clone Source**

Both build and example deployment files are stored within this repo.  For any of the below deployment methods, start here:

```shell
git clone https://github.com/google/trillian.git/
cd trillian
```

## Local Deployments

**Run With Docker Compose**

For simple deployments, running in a container is an easy way to get up and running with a local database.  To use Docker to run and interact with Trillian, start here:

Set a random password and bring up the services defined in the provided compose file.  This includes a local MySQL database, a one-shot container to create the schema and the trillian server:

```shell
# Set a random password
export DB_PASSWORD="$(openssl rand -hex 16)"

# Bring up services defined in this compose file.  This includes:
# - local MySQL database
# - container to initialize the database
# - the trillian server
docker-compose -f examples/deployment/docker-compose.yml up
```

Verify that your local installation is working by checking the metrics endpoint:

```shell
curl localhost:8091/metrics
```

## Cloud Deployments

For better persistence and performance you may want to run in your datacenter or a cloud.  Here are some simple cloud deployment templates:

### Run in GCP

TODO

### Run in AWS

With a pair of AWS keys [accessible to Terraform](https://www.terraform.io/docs/providers/aws/), this template deploys a simple Trillian setup in AWS using EC2 and RDS MySQL.

```shell
cd examples/deployment/aws/

# Set a random password
export TF_VAR_DB_PASSWORD="$(openssl rand -hex 16)"
# Substitute this variable with a block you'll be accessing from
export TF_VAR_WHITELIST_CIDR="0.0.0.0/0"

# Review and Create Resources
terraform plan
terraform apply
```
