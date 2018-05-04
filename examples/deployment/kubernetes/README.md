Deploying onto Kubernetes in Google Cloud
=========================================

This document guides you through the process of spinning up an example Trillian
deployment on Google Cloud using Kubernetes and Cloud Spanner.

Prerequisites
-------------

1. You should have this repo checked out :)
1. A recent debian distribution (other platforms may work, but YMMV)
1. You must have the `jq` binary installed
1. You have a Google account with billing configured

Process
-------
1. Go to [Google Cloud Console](https://console.cloud.google.com)
1. Create a new project
1. Edit the [config.sh](config.sh) file, set `PROJECT_NAME` to the name of your
   project
1. run: `./create.sh`.
   This script will create the Kubernetes cluster, node pools, and Spanner
   database, service account and etcd cluster.
   It should take about 5 to 10 minutes to finish and must complete without
   error.
1. Now you can deploy the trillian services.
   Run: `./deploy.sh`
   This will build the trillian docker images, tag them, and create/update the
   Kubernetes deployment.
1. To update a running deployment, simply re-run `./deploy.sh` at any time.

Next steps
----------
You should now have a working Trilian Log deployment in Kubernetes.
To do something useful with it, you'll need to run a "personality" layer, and
provision one or more trees into the Trillian log.

To provision a tree into Trillian, you can use the Trillian admin API, like so:

```bash
curl -X POST ${LOG_URL}/v1beta1/trees -d '{ "tree":{ "tree_state":"ACTIVE", "tree_type":"LOG", "hash_strategy":"RFC6962_SHA256", "signature_algorithm":"ECDSA", "max_root_duration":"0", "hash_algorithm":"SHA256" }, "key_spec":{ "ecdsa_params":{ "curve":"P256" } } }'
```

The easiest way to do this is probably to use `kubectl exec <name of one of the logserver pods> -ti -- /bin/bash` to get a shell on a logserver POD, and use curl from there.

(Use `kubectl get pods` to retrieve a list of all the PODs.)

**NOTE: none of the Trillian APIs are exposed to the internet with this config,
this is intentional since the only access to Trillian should be via a
personality layer.**

Next, you may wish to deploy the [Certificate Transparency personality](https://github.com/google/certificate-transparency-go/tree/master/trillian).
Example Kubernetes deployment configs for that can be found [here](https://github.com/google/certificate-transparency-go/tree/master/trillian/examples/deployment/kubernetes).


Known Issues
------------
- This deployment is quite tightly coupled to Google Cloud Kubernetes
- Only CloudSpanner is supported currently
- There is no Trillian Map support currently (because there is no map support
  in the current CloudSpanner storage implementation.
