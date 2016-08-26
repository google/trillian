package storage

//go:generate sh -c "cd $GOPATH/src && protoc --go_out=plugins=grpc:. github.com/google/trillian/storage/*.proto"

//go:generate mockgen -self_package github.com/google/trillian/storage -package storage -destination mock_storage.go -imports=trillian=github.com/google/trillian github.com/google/trillian/storage LogTX,MapTX,ReadOnlyLogTX,ReadOnlyMapTX,MapStorage,LogStorage
