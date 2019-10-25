# Deploying onto Kubernetes in Google Cloud

This document guides you through the process of spinning up an example Trillian
deployment on Google Cloud using Kubernetes and Cloud Spanner.

## Architecture

The infrastructure is created using Terraform. It consists of a Cloud Spanner
database, a Trillian-specific etcd cluster, the Trillian logservice and
logsigner. A Workload Identity is setup by Terraform to give the services access
to Cloud Spanner. The container images are built using Google Cloud Build.

## Prerequisites

1. You should have this repo checked out :)
1. A recent [Debian](https://debian.org) based distribution (other platforms
   may work, but YMMV)
1. You have `terraform`, `kubectl` and `gcloud` installed (See
   [Cloud quickstart](https://cloud.google.com/kubernetes-engine/docs/quickstart)
   docs)
1. You have a Google account with billing configured
1. You may need to request additional Quota for Compute Engine "in-use IP addresses" (need >= 11)
[link](https://console.cloud.google.com/iam-admin/quotas?service=compute.googleapis.com&metric=In-use%20IP%20addresses)


## Process

1. Go to [Google Cloud Console](https://console.cloud.google.com)
1. Create a new project and copy its [Project
   ID](https://cloud.google.com/resource-manager/docs/creating-managing-projects#identifying_projects).
1. Create the infrastructure: `terraform init && terraform apply -var="gcp_project=PROJECT_ID"`.
   It is ok to re-execute this command at any time.
1. The script will output a gcloud command similar to:
   ```shell
   gcloud config set project PROJECT_ID && \
   gcloud container clusters get-credentials cluster --region=us-west1 && \
   gcloud builds submit --config=cloudbuild.yaml ../../..
   ```
   Execute this set of commands to get the credentials for your new cluster and build
   the docker images.
1. Deploy the containers: `kubectl apply -k .`

You should now have a working Trillian Log deployment in Kubernetes.

**NOTE: none of the Trillian APIs are exposed to the internet with this config,
this is intentional since the only access to Trillian should be via a
personality layer.**

## Next steps

To do something useful with the deployment, you'll need provision one or more
trees into the Trillian log, and run a "personality" layer.

To provision a tree into Trillian, use the `provision_tree.sh` script (which
uses `kubectl` to forward requests to the Trillian Log's admin API):

```bash
./provision_tree.sh example-config.sh
```

Make a note of the tree ID for the new tree.

Next, you may wish to deploy the
[Certificate Transparency personality](https://github.com/google/certificate-transparency-go/tree/master/trillian).
The CT repo includes Kubernetes
[instructions](https://github.com/google/certificate-transparency-go/tree/master/trillian/examples/deployment/kubernetes/README.md)
and
[deployment configurations](https://github.com/google/certificate-transparency-go/tree/master/trillian/examples/deployment/kubernetes/).

## Uninstall

`terraform destroy`

## Known Issues

- This deployment is quite tightly coupled to Google Cloud Kubernetes
- Only CloudSpanner is supported currently
- There is no Trillian Map support currently (because there is no map support
  in the current CloudSpanner storage implementation).
