package main

import (
	"flag"
	"fmt"
	"net"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/trillian/server"
	"google.golang.org/grpc"
	"github.com/google/trillian"
	"github.com/google/trillian/storage/tools"
)

// TODO: Move this code out to a better place when we tidy up the initial test main stuff
// It's just a basic skeleton at the moment.
func main() {
	flag.Parse()

	treeId := tools.GetLogIdFromFlagsOrDie()
	port := tools.GetLogServingPort()
	storage := tools.GetStorageFromFlagsOrDie(treeId)
	logServer := server.NewTrillianLogServer(&storage)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))

	if err != nil {
		panic(fmt.Sprintf("Failed to listen on port %d", port))
	}

	grpcServer := grpc.NewServer()
	trillian.RegisterTrillianLogServer(grpcServer, logServer)
	grpcServer.Serve(lis)
}
