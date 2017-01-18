package integration

import (
	"flag"
	"testing"

	"github.com/google/trillian"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var server = flag.String("map_rpc_server", "localhost:8091", "Server address:port")
var mapID = flag.Int64("map_id", -1, "Trillian MapID to use for test")

func getClient() (*grpc.ClientConn, trillian.TrillianMapClient, error) {
	conn, err := grpc.Dial(*server, grpc.WithInsecure())
	if err != nil {
		return nil, nil, err
	}
	return conn, trillian.NewTrillianMapClient(conn), nil
}

func TestMapIntegration(t *testing.T) {
	flag.Parse()
	if *mapID == -1 {
		t.Skip("Map integration test skipped as no map ID provided")
	}

	conn, client, err := getClient()
	if err != nil {
		t.Fatalf("Failed to get map client: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()
	if err := RunMapIntegration(ctx, *mapID, client); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}
