package trillian

//go:generate sh -c "cd $GOPATH/src && protoc --go_out=plugins=grpc:. github.com/google/trillian/*proto"

//go:generate mockgen -self_package github.com/google/trillian -package trillian -destination mock_log_client.go github.com/google/trillian TrillianLogClient,TrillianLogServer,TrillianMapClient,TrillianMapServer
