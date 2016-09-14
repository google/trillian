package ct_mapper

//go:generate sh -c "cd $GOPATH/src && protoc --go_out=plugins=grpc:. github.com/google/trillian/examples/ct/ct_mapper/*proto"
