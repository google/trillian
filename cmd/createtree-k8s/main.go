// Copyright 2023 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main contains the implementation and entry point for the
// creating a tree in Trillian and adding it into a ConfigMap.
// More details and use cases are in:
// ../../examples/deployment/kubernetes/README-SCAFFOLDING.md
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/client/rpcflags"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/signals"
	"sigs.k8s.io/release-utils/version"
)

const (
	// Key in the configmap holding the value of the tree.
	treeKey = "treeID"
)

var (
	ns              = flag.String("namespace", "", "Namespace where to update the configmap in")
	cmname          = flag.String("configmap", "", "Name of the configmap where the treeID lives")
	adminServerAddr = flag.String("admin_server", "log-server.trillian-system.svc:80", "Address of the gRPC Trillian Admin Server (host:port)")
	treeState       = flag.String("tree_state", trillian.TreeState_ACTIVE.String(), "State of the new tree")
	treeType        = flag.String("tree_type", trillian.TreeType_LOG.String(), "Type of the new tree")
	displayName     = flag.String("display_name", "", "Display name of the new tree")
	description     = flag.String("description", "", "Description of the new tree")
	maxRootDuration = flag.Duration("max_root_duration", time.Hour, "Interval after which a new signed root is produced despite no submissions; zero means never")
	force           = flag.Bool("force", false, "Force create a new tree and update configmap")
)

func main() {
	flag.Parse()
	ctx := signals.NewContext()
	if *ns == "" {
		logging.FromContext(ctx).Fatal("Need to specify --namespace for where to update the configmap in")
	}
	if *cmname == "" {
		logging.FromContext(ctx).Fatal("Need to specify --configmap for which configmap to update")
	}

	versionInfo := version.GetVersionInfo()
	logging.FromContext(ctx).Infof("running Version: %s GitCommit: %s BuildDate: %s", versionInfo.GitVersion, versionInfo.GitCommit, versionInfo.BuildDate)

	config, err := rest.InClusterConfig()
	if err != nil {
		logging.FromContext(ctx).Fatalf("Failed to get InClusterConfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logging.FromContext(ctx).Fatalf("Failed to get clientset: %v", err)
	}
	cm, err := clientset.CoreV1().ConfigMaps(*ns).Get(ctx, *cmname, metav1.GetOptions{})
	if err != nil {
		logging.FromContext(ctx).Fatalf("Failed to get the configmap %s/%s: %v", *ns, *cmname, err)
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	if treeID, ok := cm.Data[treeKey]; ok && !*force {
		logging.FromContext(ctx).Infof("Found existing TreeID: %s", treeID)
		return
	}

	tree, err := createTree(ctx)
	if err != nil {
		logging.FromContext(ctx).Fatalf("Failed to create the trillian tree: %v", err)
	}
	cm.Data[treeKey] = fmt.Sprint(tree.TreeId)
	logging.FromContext(ctx).Infof("Created a new tree %d updating configmap %s/%s", tree.TreeId, *ns, *cmname)

	_, err = clientset.CoreV1().ConfigMaps(*ns).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		logging.FromContext(ctx).Fatalf("Failed to update the configmap: %v", err)
	}
}

func createTree(ctx context.Context) (*trillian.Tree, error) {
	req, err := newRequest(ctx)
	if err != nil {
		return nil, err
	}

	dialOpts, err := rpcflags.NewClientDialOptionsFromFlags()
	if err != nil {
		return nil, errors.Wrap(err, "failed to determine dial options")
	}

	conn, err := grpc.Dial(*adminServerAddr, dialOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to dial")
	}
	defer conn.Close()

	adminClient := trillian.NewTrillianAdminClient(conn)
	logClient := trillian.NewTrillianLogClient(conn)

	return client.CreateAndInitTree(ctx, req, adminClient, logClient)
}

func newRequest(ctx context.Context) (*trillian.CreateTreeRequest, error) {
	ts, ok := trillian.TreeState_value[*treeState]
	if !ok {
		return nil, fmt.Errorf("unknown TreeState: %v", *treeState)
	}

	tt, ok := trillian.TreeType_value[*treeType]
	if !ok {
		return nil, fmt.Errorf("unknown TreeType: %v", *treeType)
	}

	ctr := &trillian.CreateTreeRequest{Tree: &trillian.Tree{
		TreeState:       trillian.TreeState(ts),
		TreeType:        trillian.TreeType(tt),
		DisplayName:     *displayName,
		Description:     *description,
		MaxRootDuration: durationpb.New(*maxRootDuration),
	}}
	logging.FromContext(ctx).Infof("Creating Tree: %+v", ctr.Tree)

	return ctr, nil
}
