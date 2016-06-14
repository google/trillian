package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage/tools"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// TODO: Move this code out to a better place when we tidy up the initial test main stuff
// It's just a basic skeleton at the moment.
func main() {
	flag.Parse()

	port := tools.GetLogServerPort()

	// TODO: Other options apart from insecure connections
	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithInsecure(), grpc.WithTimeout(time.Second*5))

	if err != nil {
		panic(err)
	}

	defer conn.Close()

	client := trillian.NewTrillianLogClient(conn)

	// Just request leaf 1 from tree 1 and print the result
	getLeafByIndexResponse, err := client.GetLeavesByIndex(context.Background(), &trillian.GetLeavesByIndexRequest{LogId: proto.Int64(1), LeafIndex: []int64{1}})

	if err != nil {
		fmt.Printf("Got error in call: %v", err)
	} else {
		fmt.Printf("Got server response: %v", getLeafByIndexResponse)
	}
}
