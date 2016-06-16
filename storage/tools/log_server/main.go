package main

import (
	"flag"
	"fmt"
	"net"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/server"
	"github.com/google/trillian/storage/tools"
	"google.golang.org/grpc"
)

// TODO: Move this code out to a better place when we tidy up the initial test main stuff
// It's just a basic skeleton at the moment.
func main() {
	flag.Parse()

	port := tools.GetLogServerPort()
	provider := tools.GetLogStorageProviderFromFlags()
	logServer := server.NewTrillianLogServer(provider)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))

	if err != nil {
		panic(fmt.Sprintf("Failed to listen on port %d", port))
	}

	grpcServer := grpc.NewServer()
	trillian.RegisterTrillianLogServer(grpcServer, logServer)
	glog.Infof("Server starting on port %d", port)
	grpcServer.Serve(lis)
}
