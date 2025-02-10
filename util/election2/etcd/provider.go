package etcd

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/trillian/util/election2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/klog/v2"
)

// ElectionName identifies the etcd election implementation.
const ElectionName = "etcd"

var (
	lockDir = flag.String("lock_file_path", "/test/multimaster", "etcd lock file directory path")
)

func init() {
	if err := election2.RegisterProvider(ElectionName, newFactory); err != nil {
		klog.Fatalf("Failed to register election implementation %v: %v", ElectionName, err)
	}
}

// NewFactory builds an election factory that uses the given parameters.
func newFactory() (election2.Factory, error) {
	hostname, _ := os.Hostname()
	instanceID := fmt.Sprintf("%s.%d", hostname, os.Getpid())

	servers := flag.Lookup("etcd_servers").Value.String()

	var client *clientv3.Client
	var err error
	if servers != "" {
		if client, err = clientv3.New(clientv3.Config{
			Endpoints:   strings.Split(servers, ","),
			DialTimeout: 5 * time.Second,
		}); err != nil {
			klog.Exitf("Failed to connect to etcd at %v: %v", servers, err)
		}
	}

	if client == nil {
		return nil, errors.New("--etcd_servers must be supplied to initialize etcd elections")
	}

	// The passed in etcd client should remain valid for the lifetime of the object.
	return &Factory{
		client:     client,
		instanceID: instanceID,
		lockDir:    *lockDir,
	}, nil
}
