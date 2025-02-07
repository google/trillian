//go:build etcd || !k8s

package provider

import (
	_ "github.com/google/trillian/util/election2/etcd"
)
