# Deploying Trillian

Want to deploy/use the Trillian General Transparency project in the cloud? Here
are some common ways of getting off the ground with Docker.

## Setup

**Clone Source**

Both build and example deployment files are stored within this repo. For any of
the below deployment methods, start by cloning the repo.

```shell
git clone https://github.com/google/trillian.git
cd trillian
```

## Local Deployments

**Run With Docker Compose**

For simple deployments, running in a container is an easy way to get up and
running with a local database. To use Docker to run and interact with Trillian,
start here.

Set a random password and bring up the services defined in the provided compose
file. This includes a local MySQL database, a one-shot container to create the
schema and the trillian server.

```shell
# Set a random password
export MYSQL_ROOT_PASSWORD="$(openssl rand -hex 16)"

# Bring up services defined in this compose file.  This includes:
# - local MySQL database
# - container to initialize the database
# - the trillian server
docker-compose -f examples/deployment/docker-compose.yml up
```

Verify that your local installation is working by checking the metrics endpoint.

```shell
curl localhost:8091/metrics
```

Debugging problems with Docker setup is beyond the scope of this document, but
some helpful options include:

 - Showing debug information with the `--verbose` flag.
 - Running `docker events` in a parallel session.
 - Using `docker-compose ps` to show running containers and their ports.


## Cloud Deployments

For better persistence and performance you may want to run in your datacenter or
a cloud.

### Run in GCP

Trillian can be deployed on Google Cloud Platform using
[Kubernetes](https://kubernetes.io/). We provide
[instructions](kubernetes/README.md),
[scripts and configuration files](kubernetes/) for performing a deployment.
[Daz Wilkin has written a Medium post](https://medium.com/google-cloud/trillian-on-google-cloud-platform-621a37f2431c)
based on these instructions that illustrates the steps required.

### Run in AWS

Trillian can be deployed on Amazon Web Services using
[Terraform](https://www.terraform.io/). We provide [instructions](aws/README.md)
and [configuration files](aws/) for performing a deployment.
