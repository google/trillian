package storage

//go:generate mockgen -self_package github.com/google/trillian/storage -package storage -destination mock_storage.go -imports=trillian=github.com/google/trillian,proto=github.com/google/trillian/storage/proto github.com/google/trillian/storage LogTX,MapTX,ReadOnlyLogTX,ReadOnlyMapTX,MapStorage,LogStorage
