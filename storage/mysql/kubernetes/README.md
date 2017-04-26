:exclamation: **EXPERIMENTAL** :exclamation:

# Usage #

To run a Galera MySQL cluster on Google Cloud, install the
[Cloud SDK](https://cloud.google.com/sdk/) and configure it for your project.
[Provision a Container cluster](https://cloud.google.com/container-engine/docs/clusters/operations),
then run the following command:
```
kubectl start -f galera.yaml
```

This will start the Galera cluster. You can monitor provisoning of this cluster
by visiting http://127.0.0.1:8001/ui/ after running:
```
kubectl proxy
```

This dashboard will also show the external IP of the cluster on the
"Services" page, on the row for the "mysql" service.

# Derivation #

Based on
[the mysql-galera example from the Kubernetes GitHub repository](https://github.com/kubernetes/kubernetes/tree/v1.5.4/examples/storage/mysql-galera),
which is available under
[the Apache 2.0 license](https://github.com/kubernetes/kubernetes/blob/v1.5.4/LICENSE).

The following modifications have been made:
- Increased CPU limit per replica to 2.
- Each instance will use a persistent SSD for storage of its database.
- The cluster will be managed as a
  [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/).
- The cluster is accessed via a load balancer, enabling a transparent
  multi-master setup.
- Updated image to use Percona 5.7.
- Simplified scripts by removing unnecessary options.
- Added some utility scripts:
  - image/env.sh
  - image/push.sh
- Added liveness and readiness probes to the Kubernetes config.
- Moved usernames and passwords into [Secrets](https://kubernetes.io/docs/concepts/configuration/secret/).
