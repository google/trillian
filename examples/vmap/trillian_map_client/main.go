package main

import (
	"flag"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var server = flag.String("server", "localhost:8091", "Server address:port")

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*server, grpc.WithInsecure())
	if err != nil {
		glog.Fatal(err)
	}
	defer conn.Close()

	c := trillian.NewTrillianMapClient(conn)

	key := []byte("This Is A Key")

	{
		req := &trillian.SetMapLeavesRequest{
			MapId: 1,
			KeyValue: []*trillian.KeyValue{
				{
					Key: key,
					Value: &trillian.MapLeaf{
						LeafHash:  []byte("This is a leaf hash"),
						LeafValue: []byte("This is a leaf value"),
						ExtraData: []byte("This is some extra data"),
					},
				},
			},
		}
		resp, err := c.SetLeaves(context.Background(), req)
		if err != nil {
			glog.Error(err)
			return
		}
		glog.Infof("Got SetLeaves response: %+v", resp)
	}

	{
		req := &trillian.GetMapLeavesRequest{
			MapId:    1,
			Revision: -1,
			Index: [][]byte{
				key,
			},
		}
		resp, err := c.GetLeaves(context.Background(), req)
		if err != nil {
			glog.Error(err)
		}
		glog.Infof("Got GetLeaves response: %+v", resp)
	}
}
