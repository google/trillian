Trillian on Cloud Kubernetes with Cloud Spanner
====================================================

This document outlines the steps necessary to get an instance of Trillian up
and running on Google Cloud.

 1. Create a Google Cloud project (we'll call it's project ID `${PROJECT_ID}` below)
 2. Enable APIs
    1. Kubernetes
    2. Cloud Spanner
    3. ...

 3. Create the public API key (see https://cloud.google.com/kubernetes-engine/docs/tutorials/authenticating-to-cloud-platform)
   1. From the APIs & Services > Credentials page, click Create credentials > API key.
      A key will automatically be created.
   2. Restrict API key to only have access to:
      * Google Cloud APIs
      * Cloud Spanner API
   3. Download service account key (we'll call that file `service-key.json`)
   4. run: `kubectl create secret generic spanner-key --from-file=key.json=service-key.json`

 4. Create Spanner instance & database
   1. Click on menu > Cloud Spanner
   2. Click on "Create Instance" (we'll call its instance ID `${SPANNER_INSTANCE}`)
   4. Choose region (we'll call that ${REGION})
   5. Choose number of nodes
   6. Click create Database
     1. Fill in a name (we'll call the DB instance `${DATABASE_INSTANCE}`)
     2. Click continue
     4. In `Define your database schema`, click the `Edit as text` slider
     3. paste contents of [spanner.sd](storage/cloudspanner/spanner.sdl) into the text box (you may need to remove the SQL comments prefixed with `--` at the top)
     4. Click on create

 5. Create kubernetes cluster
   1. menu > Kubernetes
   2. click on Create Cluster
   3. Set cluster name to something (we'll call this `${CLUSTER_NAME}`)
   4. Set zone to something inside ${REGION}
   5. Click create

 6. Start initial jobs
   1. Edit [scripts/deploy_gce.sh](scripts/deploy.sh) and configure the environment variables for your deployment.
   1. run: `./scripts/deploy_gce.sh`


Setting up continuous integration
=============================================

Now that you have a working Trillian-on-cloud instance, you can integrate it
with CI/CD so that pushes to master update your Trillian instance.

 1. Create service account credentials
   1. In your Cloud Platform Console project, open the Credentials page.
   2. Click Create credentials > Service account key.
   3. Under Service account select New service account.
   4. Enter a Service account name, e.g. trillian-pusher-ci
   5. Under Role, select Project > Editor.
   6. Under Key type, select JSON.
   7. Click Create. The Cloud Platform Console downloads a new JSON file to your computer. The name of this file starts with your project ID.
   8. Provide the service key to the deploy script that CI runs. You might need
   the output of the following command: `base64 service-key.json | tr -d '\040\011\012\015'`
      - Ensure that _Display value in build log_ switch is set to OFF!*

