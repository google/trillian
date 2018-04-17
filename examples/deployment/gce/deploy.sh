# Script assumptions:
# - Go 1.9 is installed
export PROJECT_NAME=TODO
export TAG=latest
export LOG_URL=TODO
export MAP_URL=TODO

# Get Trillian
go get github.com/google/trillian/...
cd $GOPATH/src/github.com/google/trillian

# Build docker images
docker build -f examples/deployment/docker/db_server/Dockerfile -t us.gcr.io/$PROJECT_NAME/db:$TAG .
docker build -f examples/deployment/docker/log_server/Dockerfile -t us.gcr.io/$PROJECT_NAME/log_server:$TAG .
docker build -f examples/deployment/docker/log_signer/Dockerfile -t us.gcr.io/$PROJECT_NAME/log_signer:$TAG .
docker build -f examples/deployment/docker/map_server/Dockerfile -t us.gcr.io/$PROJECT_NAME/map_server:$TAG .

# Connect to gcloud
gcloud config set project $PROJECT_NAME
gcloud config set compute/zone us-central1-b
gcloud container clusters create "cluster-1" --machine-type "n1-standard-1" --image-type "COS" --num-nodes "2" --enable-autorepair --enable-autoupgrade
# Create some dedicated node pools, the DB and MapServer will use the cluster's default pool
gcloud container node-pools create "logserver-pool" --machine-type "n1-standard-1" --image-type "COS" --num-nodes "2" --enable-autorepair --enable-autoupgrade
gcloud container node-pools create "signer-pool" --machine-type "n1-standard-1" --image-type "COS" --num-nodes "1" --enable-autorepair --enable-autoupgrade
gcloud container node-pools create "ctfe-pool" --machine-type "n1-standard-1" --image-type "COS" --num-nodes "2" --enable-autorepair --enable-autoupgrade
gcloud container clusters get-credentials cluster-1

# Push docker images
gcloud docker -- push us.gcr.io/${PROJECT_NAME}/db:$TAG
gcloud docker -- push us.gcr.io/${PROJECT_NAME}/log_server:$TAG
gcloud docker -- push us.gcr.io/${PROJECT_NAME}/log_signer:$TAG
gcloud docker -- push us.gcr.io/${PROJECT_NAME}/map_server:$TAG

# Prepare configmap:
kubectl delete configmap deploy-config
kubectl create -f examples/deployment/kubernetes/trillian-mysql.yaml

# Work-around for etcd-operator role on GKE:
kubectl create clusterrolebinding myname-cluster-admin-binding --clusterrole=cluster-admin

# Launch with kubernetes
kubectl apply -f examples/deployment/kubernetes/.
kubectl get all
kubectl get services

# Create trees
curl -X POST ${LOG_URL}/v1beta1/trees -d '{ "tree":{ "tree_state":"ACTIVE", "tree_type":"LOG", "hash_strategy":"RFC6962_SHA256", "signature_algorithm":"ECDSA", "max_root_duration":"0", "hash_algorithm":"SHA256" }, "key_spec":{ "ecdsa_params":{ "curve":"P256" } } }'
curl -X POST ${MAP_URL}/v1beta1/trees -d '{ "tree":{ "tree_state":"ACTIVE", "tree_type":"MAP", "hash_strategy":"CONIKS_SHA512_256", "signature_algorithm":"ECDSA", "max_root_duration":"0", "hash_algorithm":"SHA256" }, "key_spec":{ "ecdsa_params":{ "curve":"P256" } } }'
