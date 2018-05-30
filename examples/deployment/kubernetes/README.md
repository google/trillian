Deploying onto Kubernetes in Google Cloud
=========================================

This document guides you through the process of spinning up an example Trillian
deployment on Google Cloud using Kubernetes and Cloud Spanner.

Prerequisites
-------------

1. You should have this repo checked out :)
1. A recent [Debian](https://debian.org) based distribution (other platforms
   may work, but YMMV)
1. You must have the [`jq` binary](https://packages.debian.org/stretch/jq)
   installed (for command-line manipulation of JSON)
1. You have `gcloud`/`kubectl` etc. installed (See
   [Cloud quickstart](https://cloud.google.com/kubernetes-engine/docs/quickstart)
   docs)
1. You have a Google account with billing configured
1. You will need to request additional Quota for Compute Engine "in-use IP addresses" (need: 11; default: 8) [link](https://console.cloud.google.com/iam-admin/quotas?service=compute.googleapis.com&metric=In-use%20IP%20addresses)

Process
-------
1. Go to [Google Cloud Console](https://console.cloud.google.com)
1. Create a new project
1. Edit the [config.sh](config.sh) file, set `PROJECT_NAME` to the name of your
   project
1. Run: `./create.sh`.
   This script will create the Kubernetes cluster, node pools, and Spanner
   database, service account and etcd cluster.
   It should take about 5 to 10 minutes to finish and must complete without
   error.
1. Now you can deploy the Trillian services.
   Run: `./deploy.sh`
   This will build the Trillian Docker images, tag them, and create/update the
   Kubernetes deployment.
1. To update a running deployment, simply re-run `./deploy.sh` at any time.

Next steps
----------
You should now have a working Trilian Log deployment in Kubernetes.
To do something useful with it, you'll need provision one or more trees into
the Trillian log, and run a "personality" layer.

To provision a tree into Trillian, you can use the Trillian admin API, like so:

```bash
curl -X POST ${LOG_URL}/v1beta1/trees -d '{ "tree":{ "tree_state":"ACTIVE", "tree_type":"LOG", "hash_strategy":"RFC6962_SHA256", "signature_algorithm":"ECDSA", "max_root_duration":"0", "hash_algorithm":"SHA256" }, "key_spec":{ "ecdsa_params":{ "curve":"P256" } } }'
{... tree_id: <large number here> ...}
curl -X POST ${LOG_URL}/v1beta1/logs/${tree_id}:init

```

The easiest way to do this is to port-forward one of the logserver pods to your local workstation `kubectl port-forward <name of one of the logserver pods> 8091:8091` and use curl to access the API endpoint:

1. Grab the name of 0th logserver pod:
```
LOG_POD=$(\
  kubectl get pods \
  --selector=io.kompose.service=trillian-log \
  --output=jsonpath='{.items[0].metadata.name}')
```
1. Port forward this pod's port `8091` to your local workstation (we're using `8091` locally too):
```
kubectl port-forward ${LOG_POD} 8091:8091
```
1. Create a Trillian tree:
```
RESPONSE=$(curl \
--silent \
--request POST \
http://localhost:8091/v1beta1/trees \
--data '{ "tree":{ "tree_state":"ACTIVE", "tree_type":"LOG", "hash_strategy":"RFC6962_SHA256", "signature_algorithm":"ECDSA", "max_root_duration":"0", "hash_algorithm":"SHA256" }, "key_spec":{ "ecdsa_params":{ "curve":"P256" } } }')
TREE_ID=$(echo ${RESPONSE} | jq --raw-output .tree_id)
echo ${RESPONSE}

```
1. Initialize it:
```
curl \
--request POST \
http://localhost:8091/v1beta1/logs/${TREE_ID}:init
```

**NOTE: none of the Trillian APIs is exposed to the Internet with this config.
This is intentional since the only access to Trillian should be via a 
personality layer.**

Next, you may wish to deploy the [Certificate Transparency personality](https://github.com/google/certificate-transparency-go/tree/master/trillian).
Example Kubernetes deployment configs for that can be found [here](https://github.com/google/certificate-transparency-go/tree/master/trillian/examples/deployment/kubernetes).
You can probably use the [deploy_gce_ci.sh](https://github.com/google/certificate-transparency-go/blob/master/scripts/deploy_gce_ci.sh)
script with a small tweak to the environment variables it contains at the top
to set the project ID and zone.

TODO(al): Provide a complete end-to-end script/walk through of this.


Known Issues
------------
- This deployment is quite tightly coupled to Google Cloud Kubernetes
- Only CloudSpanner is supported currently
- There is no Trillian Map support currently (because there is no map support
  in the current CloudSpanner storage implementation).
