package trillian

//go:generate protoc -I=. -I=$GOPATH/src/ --go_out=plugins=grpc:. trillian_api.proto trillian.proto
