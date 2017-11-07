// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// The mapper binary performs log->map mapping.
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/certificate-transparency-go/client"
	"github.com/google/certificate-transparency-go/jsonclient"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/trillian"
	"github.com/google/trillian/examples/ct/ctmapper"
	"github.com/google/trillian/examples/ct/ctmapper/ctmapperpb"
	"google.golang.org/grpc"

	pb "github.com/golang/protobuf/proto"
	ct "github.com/google/certificate-transparency-go"
)

var sourceLog = flag.String("source", "https://ct.googleapis.com/submariner", "Source CT Log")
var mapServer = flag.String("map_server", "", "host:port for the map server")
var mapID = flag.Int("map_id", -1, "Map ID to write to")
var logBatchSize = flag.Int("log_batch_size", 256, "Max number of entries to process at a time from the CT Log")

//TODO(al): factor this out into a reusable thing.

// CTMapper converts between a certificate transparency Log and a Trillian Map.
type CTMapper struct {
	mapID int64
	ct    *client.LogClient
	vmap  trillian.TrillianMapClient
}

func updateDomainMap(m map[string]ctmapperpb.EntryList, cert x509.Certificate, index int64, isPrecert bool) {
	domains := make(map[string]bool)
	if len(cert.Subject.CommonName) > 0 {
		domains[cert.Subject.CommonName] = true
	}
	for _, n := range cert.DNSNames {
		if len(n) > 0 {
			domains[n] = true
		}
	}

	for k := range domains {
		el := m[k]
		if isPrecert {
			el.PrecertIndex = append(el.PrecertIndex, index)
		} else {
			el.CertIndex = append(el.CertIndex, index)
		}
		el.Domain = k
		m[k] = el
	}
}

func (m *CTMapper) oneMapperRun(ctx context.Context) (bool, error) {
	start := time.Now()
	glog.Info("starting mapping batch")
	getRootReq := &trillian.GetSignedMapRootRequest{MapId: m.mapID}
	getRootResp, err := m.vmap.GetSignedMapRoot(context.Background(), getRootReq)
	if err != nil {
		return false, err
	}

	mapperMetadata := &ctmapperpb.MapperMetadata{}
	if getRootResp.GetMapRoot().Metadata != nil {
		var metadataProto ptypes.DynamicAny
		if err := ptypes.UnmarshalAny(getRootResp.MapRoot.Metadata, &metadataProto); err != nil {
			return false, fmt.Errorf("failed to unmarshal MapRoot.Metadata: %v", err)
		}
		mapperMetadata = metadataProto.Message.(*ctmapperpb.MapperMetadata)
	}

	startEntry := mapperMetadata.HighestFullyCompletedSeq + 1
	endEntry := startEntry + int64(*logBatchSize)

	glog.Infof("Fetching entries [%d, %d] from log", startEntry, endEntry)

	// Get the entries from the log:
	logEntries, err := m.ct.GetEntries(ctx, startEntry, endEntry)
	if err != nil {
		return false, err
	}
	if len(logEntries) == 0 {
		glog.Info("No entries from log")
		return false, nil
	}

	// figure out which domains we've found:
	domains := make(map[string]ctmapperpb.EntryList)
	for _, entry := range logEntries {
		if entry.Leaf.LeafType != ct.TimestampedEntryLeafType {
			glog.Info("Skipping unknown entry type %v at %d", entry.Leaf.LeafType, entry.Index)
			continue
		}
		if entry.Index > mapperMetadata.HighestFullyCompletedSeq {
			mapperMetadata.HighestFullyCompletedSeq = entry.Index
		}
		switch entry.Leaf.TimestampedEntry.EntryType {
		case ct.X509LogEntryType:
			cert, err := x509.ParseCertificate(entry.Leaf.TimestampedEntry.X509Entry.Data)
			if err != nil {
				glog.Warningf("Can't parse cert at index %d, continuing anyway because this is a toy", entry.Index)
				continue
			}
			updateDomainMap(domains, *cert, entry.Index, false)
		case ct.PrecertLogEntryType:
			precert, err := x509.ParseTBSCertificate(entry.Leaf.TimestampedEntry.PrecertEntry.TBSCertificate)
			if err != nil {
				glog.Warningf("Can't parse precert at index %d, continuing anyway because this is a toy", entry.Index)
				continue
			}
			updateDomainMap(domains, *precert, entry.Index, true)
		default:
			glog.Infof("Unknown logentry type at index %d", entry.Index)
			continue
		}
	}

	glog.Infof("Found %d unique domains from certs", len(domains))
	glog.Info("Fetching current map values for domains...")

	// Fetch the current map values for those domains:
	getReq := &trillian.GetMapLeavesRequest{
		MapId:    m.mapID,
		Index:    make([][]byte, 0, len(domains)),
		Revision: -1,
	}
	for d := range domains {
		getReq.Index = append(getReq.Index, ctmapper.HashDomain(d))
	}

	getResp, err := m.vmap.GetLeaves(context.Background(), getReq)
	if err != nil {
		return false, err
	}
	//glog.Info("Get resp: %v", getResp)

	proofs := 0
	for _, v := range getResp.MapLeafInclusion {
		e := ctmapperpb.EntryList{}
		if len(v.Inclusion) > 0 {
			proofs++
		}
		if err := pb.Unmarshal(v.Leaf.LeafValue, &e); err != nil {
			return false, err
		}
		glog.Infof("Got %#v", e)
		el := domains[e.Domain]
		pb.Merge(&el, &e)
		domains[e.Domain] = el
		glog.Infof("will update for %s", e.Domain)
	}
	glog.Infof("Got %d values, and %d proofs", len(getResp.MapLeafInclusion), proofs)

	glog.Info("Storing updated map values for domains...")
	// Store updated map values:
	setReq := &trillian.SetMapLeavesRequest{
		MapId:  m.mapID,
		Leaves: make([]*trillian.MapLeaf, 0, len(domains)),
	}
	for k, v := range domains {
		index := ctmapper.HashDomain(k)
		b, err := pb.Marshal(&v)
		if err != nil {
			return false, err
		}
		setReq.Leaves = append(setReq.Leaves, &trillian.MapLeaf{
			Index:     index,
			LeafValue: b,
		})
	}

	var metaAny *any.Any
	if metaAny, err = ptypes.MarshalAny(mapperMetadata); err != nil {
		return false, fmt.Errorf("failed to marshal mapper metadata as 'any': err %v", err)
	}

	setReq.Metadata = metaAny

	setResp, err := m.vmap.SetLeaves(context.Background(), setReq)
	if err != nil {
		return false, err
	}
	glog.Infof("Set resp: %v", setResp)
	d := time.Since(start)
	glog.Infof("Map run complete, took %.1f secs to update %d values (%0.2f/s)", d.Seconds(), len(setReq.Leaves), float64(len(setReq.Leaves))/d.Seconds())
	return true, nil
}

func main() {
	flag.Parse()
	conn, err := grpc.Dial(*mapServer, grpc.WithInsecure())
	if err != nil {
		glog.Fatal(err)
	}
	defer conn.Close()

	ctClient, err := client.New(*sourceLog, nil, jsonclient.Options{})
	if err != nil {
		glog.Exitf("Failed to create CT client: %v", err)
	}
	mapper := CTMapper{
		mapID: int64(*mapID),
		ct:    ctClient,
		vmap:  trillian.NewTrillianMapClient(conn),
	}
	ctx := context.Background()

	for {
		moreToDo, err := mapper.oneMapperRun(ctx)
		if err != nil {
			glog.Warningf("mapper run failed: %v", err)
		}
		if !moreToDo {
			time.Sleep(5 * time.Second)
		}

	}
}
