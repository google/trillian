package proto

//go:generate sh -c "cd $GOPATH/src && protoc --go_out=plugins=grpc:. github.com/google/trillian/storage/proto/*.proto"
