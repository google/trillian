package provider

import (
	"slices"

	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"

	_ "github.com/google/trillian/util/election2/etcd"
	_ "github.com/google/trillian/util/election2/k8s"
)

var (
	DefaultQuotaSystem   string
	DefaultStorageSystem string
)

func init() {
	defaultProvider := "mysql"
	providers := storage.Providers()
	if len(providers) > 0 && !slices.Contains(providers, defaultProvider) {
		slices.Sort(providers)
		defaultProvider = providers[0]
	}
	DefaultStorageSystem = defaultProvider

	providers = quota.Providers()
	if len(providers) > 0 && !slices.Contains(providers, defaultProvider) {
		slices.Sort(providers)
		defaultProvider = providers[0]
	}
	DefaultQuotaSystem = defaultProvider
}
