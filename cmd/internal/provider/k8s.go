//go:build k8s || !etcd

package provider

import (
	_ "github.com/google/trillian/util/election2/k8s"
)
