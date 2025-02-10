package k8s

import (
	"flag"
	"fmt"
	"time"

	"github.com/google/trillian/util/election2"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// ElectionName identifies the kubernetes election implementation.
const ElectionName = "k8s"

var (
	kubeconfig = flag.String("kubeconfig", "", "Paths to a kubeconfig. Only required if out-of-cluster.")
	namespace  = flag.String("lock_namespace", "", "The lease lock resource namespace. Only effective for election_system=k8s.")
	instanceID = flag.String("lock_holder_identity", "", "The identity of the holder of a current lease")
)

func init() {
	if err := election2.RegisterProvider(ElectionName, newFactory); err != nil {
		klog.Fatalf("Failed to register election implementation %v: %v", ElectionName, err)
	}
}

// NewFactory builds an election factory that uses the given parameters.
func newFactory() (election2.Factory, error) {
	var instance = *instanceID
	if instance == "" {
		instance = "trillian-logsigner-" + string(uuid.NewUUID())
	}

	if *namespace == "" {
		return nil, fmt.Errorf("namespace for lease lock need to be configured")
	}

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}

	clientset := kubernetes.NewForConfigOrDie(config)

	holdInterval := flag.Lookup("master_hold_interval").Value
	holdIntervalDuration, err := time.ParseDuration(holdInterval.String())
	if err != nil {
		return nil, fmt.Errorf("master_hold_interval: %w", err)
	}

	holdJitter := flag.Lookup("master_hold_jitter").Value
	holdJitterDuration, err := time.ParseDuration(holdJitter.String())
	if err != nil {
		return nil, fmt.Errorf("master_hold_jitter: %w", err)
	}

	return &Factory{
		client:        clientset.CoordinationV1(),
		namespace:     *namespace,
		instanceID:    instance,
		leaseDuration: holdJitterDuration,
		retryPeriod:   holdIntervalDuration,
	}, nil
}
