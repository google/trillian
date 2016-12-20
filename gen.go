package trillian

//go:generate sh -c "cd $GOPATH/src && protoc --go_out=plugins=grpc:. github.com/google/trillian/trillian_api.proto github.com/google/trillian/trillian.proto"
