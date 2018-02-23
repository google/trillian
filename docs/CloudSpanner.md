Continuous integration with Trillian on CloudSpanner
====================================================

This document outlines the steps necessary to get an instance of Trillian up
and running on Google Cloud, and hook it up with Travis to update the running
instance(s) with build artifacts from commited PRs.

[This](https://cloud.google.com/solutions/continuous-delivery-with-travis-ci)
document was used as a basis.

 1. Create a Google Cloud project (we'll use the name "trillian-opensource-ci" below)
 2. Create service account credentials
   1. In your Cloud Platform Console project, open the Credentials page.
   2. Click Create credentials > Service account key.
   3. Under Service account select New service account.
   4. Enter a Service account name such as continuous-integration-test.
   5. Under Role, select Project > Editor.
   6. Under Key type, select JSON.
   7. Click Create. The Cloud Platform Console downloads a new JSON file to your computer. The name of this file starts with your project ID.

3. Create the public API key
   1. From the same Credentials page, click Create credentials > API key.
      A key will automatically be created.
   2. Restrict API key to only have access to:
      * Google Cloud APIs
      * Cloud Spanner API
   3. Download service account key (we'll call that file `service-key.json`)
   4. Run the following command:
         `base64 service-key.json | tr -d '\040\011\012\015'`
      This generates a blob of base 64 which we'll use later on.
4. Enable APIs
   1. Kuberneter
   2. Cloud Spanner
   3. ...
5. Create Spanner instance & database
   1. Click on menu > Cloud Spanner
   2. Click on "Create Instance"
   3. Use instance id "trillian-opensource-ci"
   4. Choose region (e.g. us-central1)
   5. Choose number of nodes
   6. Click create Database
     1. Use DB name "trillian-opensource-ci-db"
     2. Click continue
     4. In `Define your database schema`, click the `Edit as text` slider
     3. paste contents of `storage/cloudspanner/spanner.sdl` into the text box (you may need to remove the SQL comments prefixed with `--` at the top)
     4. Click on create

 6. Create kubernetes cluster
   1. menu > Kubernetes
   2. click on Create Cluster
   3. Set cluster name to trillian-opensource-ci
   4. Set zone to us-central1-a
   5. Click create

 - make app credential for kubernetes cluster (https://cloud.google.com/kubernetes-engine/docs/tutorials/authenticating-to-cloud-platform)
  - credentials
  - create service account, name trillian-database-writer role: Cloud Spanner Database User
  - import creds as kubernetes secret (see doc)
     - kubectl create secret generic spanner-key --from-file=key.json=<path to creds file>
  





Exit the editor.
 1. Enable Spanner API


