# Deploying onto Kubernetes

This document guides you through the process of spinning up an example Trillian
deployment on Kubernetes cluster with [Knative](https://knative.dev/docs/)
installed. It's suitable for GitHub action based e2e tests
as well as local testing using something like kind cluster.

## Prerequisites

1. You should have this repo checked out :)
1. A Kubernetes cluster with [Knative](https://knative.dev/docs/) installed. One
example for installing local kind cluster with Knative is
[here](https://github.com/sigstore/scaffolding/blob/main/getting-started.md#running-locally-on-kind)
and ignoring the sigstore parts after installing the cluster.
1. [ko](https://github.com/google/ko) installed.
1. You have `kubectl` installed.

## Process

1. Create the scaffolding parts of the Trillian system.
```shell
kubectl apply -Rf ./examples/deployment/kubernetes/scaffolding
```
This spins up a namespace `trillian-system` where it deploys
the following components:
  * log-signer
  * log-server
  * mysql server
2. Let's make sure everything comes up ready:
```shell
kubectl wait -n trillian-system --for=condition=Ready --timeout=5m ksvc --all
```

You should see something like this:
```shell
vaikas@villes-mbp scaffolding % kubectl wait -n trillian-system --for=condition=Ready --timeout=5m ksvc --all
service.serving.knative.dev/log-server condition met
service.serving.knative.dev/log-signer condition met
```
3. Then create a tree in the Trillian:
```shell
ko apply -BRf ./examples/deployment/kubernetes/createtree
```

And make sure it completes.

```shell
kubectl wait -n createtree --for=condition=Complete --timeout=5m jobs createtree
```

4. Check out the tree:
```shell
kubectl -n createtree get cm  trillian-tree -ojsonpath='{.data.treeID}'
```

You should see something like this:
```shell
vaikas@villes-mbp scaffolding % kubectl -n createtree get cm  trillian-tree -ojsonpath='{.data.treeID}'
5213139395739357930%
```

5. You can then use the TreeID for example to run CTLog on
top of newly created Trillian.

6. TODO: Add examples for talking to Trillian for other things.
