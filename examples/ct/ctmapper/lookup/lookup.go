package main

import (
	"context"
	"crypto/sha256"
	"flag"

	"github.com/golang/glog"
	pb "github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/examples/ct/ctmapper/ctmapperpb"
	"google.golang.org/grpc"
)

var mapServer = flag.String("map_server", "", "host:port for the map server")
var mapID = flag.Int("map_id", -1, "Map ID to write to")

// HashDomain converts a domain into a map index.
func HashDomain(key string) []byte {
	h := sha256.New()
	h.Write([]byte(key))
	return h.Sum(nil)
}

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		glog.Info("Usage: lookup [domain <domain> ...]")
		return
	}

	conn, err := grpc.Dial(*mapServer, grpc.WithInsecure())
	if err != nil {
		glog.Fatal(err)
	}
	defer conn.Close()

	mapID := int64(*mapID)
	vmap := trillian.NewTrillianMapClient(conn)

	for i := 0; i < flag.NArg(); i++ {
		domain := flag.Arg(i)
		req := &trillian.GetMapLeavesRequest{
			MapId:    mapID,
			Index:    [][]byte{HashDomain(domain)},
			Revision: -1,
		}
		resp, err := vmap.GetLeaves(context.Background(), req)
		if err != nil {
			glog.Warning("Failed to lookup domain %s: %v", domain, err)
			continue
		}
		for _, kv := range resp.IndexValueInclusion {
			el := ctmapperpb.EntryList{}
			v := kv.IndexValue.Value.LeafValue
			if len(v) == 0 {
				continue
			}
			if err := pb.Unmarshal(v, &el); err != nil {
				glog.Warning("Failed to unmarshal leaf %s: %v", kv.IndexValue.Value.LeafValue, err)
				continue
			}
			glog.Infof("Found %s with certs at indices %v and pre-certs at indices %v", el.Domain, el.CertIndex, el.PrecertIndex)
		}
	}
}
