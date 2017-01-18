package storagepb

//go:generate protoc -I=. -I=$GOPATH/src/ --go_out=plugins=grpc:. storage.proto
