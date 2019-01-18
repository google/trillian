package server

import (
	"cloud.google.com/go/bigquery"
	"github.com/golang/glog"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/bigquery"
)

func init() {
	if err := RegisterStorageProvider("bigquery", newBigQueryStorageProvider); err != nil {
		panic(err)
	}
}

type bigQueryProvider struct {
	client *bigquery.Client
}

func optionsFromFlags() ...bigquery.ClientOption {
	o := []bigquery.ClientOption{}
	return o
}

func newBigQueryStorageProvider(mf monitoring.MetricFactory) (StorageProvider, error) {
	csMu.Lock()
	defer csMu.Unlock()

	if csStorageInstance != nil {
		return csStorageInstance, nil
	}

	client, err := bigquery.NewClient(context.TODO(), projectId, optionsFromFlags())
	if err != nil {
		return nil, err
	}
	csStorageInstance = &bigQueryProvider{
		client: client,
	}
	return csStorageInstance, nil
}

func (s *bigQueryProvider) LogStorage() storage.LogStorage {
	opts := bigquery.LogStorageOptions{}
	return bigquery.NewLogStorageWithOpts(s.client, opts)
}

func (s *bigQueryProvider) MapStorage() storage.MapStorage {
	opts := bigquery.MapStorageOptions{}
	return bigquery.NewMapStorageWithOpts(s.client, opts)
}

func (s *bigQueryProvider) AdminStorage() storage.AdminStorage {
	return bigquery.NewAdminStorage(s.client)
}

func (s *bigQueryProvider) Close() error {
	s.client.Close()
	return nil
}