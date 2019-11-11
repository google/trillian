# Trillian supported deployments

This directory contains the supported deployment types for Trillian. Currently,
this includes:

- [Kubernetes on GCP using Cloud
  Spanner](#kubernetes-on-gcp-using-cloud-spanner)

Further deployment types (community-supported) can be found in the
examples/deployment directory.

# Kubernetes on GCP using Cloud Spanner

## Architecture

The infrastructure is created using Terraform. It consists of a Cloud Spanner
database, a Trillian-specific etcd cluster, the Trillian logserver and
logsigner. A Workload Identity is set up by Terraform to give the services access
to Cloud Spanner. The container images are based on v1.3.3.

## Prerequisites
1. [Terraform](https://www.terraform.io/), [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/),
   and [gcloud](https://cloud.google.com/sdk/gcloud/) are installed.
1. You have a Google account with billing configured.
1. Terraform should have access to the credentials to manage your Cloud Project. Follow the
   instructions from the [Terraform Google Provider documentation](https://www.terraform.io/docs/providers/google/guides/provider_reference.html#full-reference) to set this up.

## Installation
1. Create a new project and copy its [Project
   ID](https://cloud.google.com/resource-manager/docs/creating-managing-projects#identifying_projects).
1. Create the infrastructure: `terraform init && terraform apply -var="gcp_project=PROJECT_ID"`.
   It is ok to re-execute this command at any time.
1. The script will output a gcloud command similar to:
   ```shell
   gcloud config set project PROJECT_ID && \
   gcloud container clusters get-credentials cluster --region=us-west1
   ```
   Execute this set of commands to get the credentials for your new cluster.
1. Deploy the containers: `kubectl apply -k .` This last command may fail with
   an [error](https://github.com/google/trillian/issues/1820) similar to:
   `unable to recognize ".": no matches for kind "EtcdCluster" in version "etcd.database.coreos.com/v1beta2"`.
   In this case, simply re-run it until the error is not reported anymore.

## Uninstall
1. Destroy the infrastructure: `terraform destroy`.

