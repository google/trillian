package ctmapperpb

//go:generate protoc -I=. -I=$GOPATH/src/ --go_out=plugins=grpc:. ct_mapper.proto
