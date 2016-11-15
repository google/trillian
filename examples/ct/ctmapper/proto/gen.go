package mapperpb

//go:generate sh -c "cd $GOPATH/src && protoc --go_out=plugins=grpc:. github.com/google/trillian/examples/ct/ctmapper/proto/*proto"
