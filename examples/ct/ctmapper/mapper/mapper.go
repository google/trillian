package main

import (
	"flag"
	"time"

	"github.com/golang/glog"
	pb "github.com/golang/protobuf/proto"
	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/client"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian"
	mapperpb "github.com/google/trillian/examples/ct/ctmapper/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
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

func updateDomainMap(m map[string]mapperpb.EntryList, cert x509.Certificate, index int64, isPrecert bool) {
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

func (m *CTMapper) oneMapperRun() (bool, error) {
	start := time.Now()
	glog.Info("starting mapping batch")
	getRootReq := &trillian.GetSignedMapRootRequest{MapId: m.mapID}
	getRootResp, err := m.vmap.GetSignedMapRoot(context.Background(), getRootReq)
	if err != nil {
		return false, err
	}
	meta := getRootResp.MapRoot.Metadata

	startEntry := int64(0)

	if meta == nil {
		meta = &trillian.MapperMetadata{}
	}
	startEntry = meta.HighestFullyCompletedSeq + 1
	endEntry := startEntry + int64(*logBatchSize)

	glog.Infof("Fetching entries [%d, %d] from log", startEntry, endEntry)

	// Get the entries from the log:
	logEntries, err := m.ct.GetEntries(startEntry, endEntry)
	if err != nil {
		return false, err
	}
	if len(logEntries) == 0 {
		glog.Info("No entries from log")
		return false, nil
	}

	// figure out which domains we've found:
	domains := make(map[string]mapperpb.EntryList)
	for _, entry := range logEntries {
		if entry.Leaf.LeafType != ct.TimestampedEntryLeafType {
			glog.Info("Skipping unknown entry type %v at %d", entry.Leaf.LeafType, entry.Index)
			continue
		}
		if entry.Index > meta.HighestFullyCompletedSeq {
			meta.HighestFullyCompletedSeq = entry.Index
		}
		switch entry.Leaf.TimestampedEntry.EntryType {
		case ct.X509LogEntryType:
			cert, err := x509.ParseCertificate(entry.Leaf.TimestampedEntry.X509Entry)
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
		Key:      make([][]byte, 0, len(domains)),
		Revision: -1,
	}
	for k := range domains {
		getReq.Key = append(getReq.Key, []byte(k))
	}

	getResp, err := m.vmap.GetLeaves(context.Background(), getReq)
	if err != nil {
		return false, err
	}
	//glog.Info("Get resp: %v", getResp)

	proofs := 0
	for _, v := range getResp.KeyValue {
		e := mapperpb.EntryList{}
		if len(v.Inclusion) > 0 {
			proofs++
		}
		if err := pb.Unmarshal(v.KeyValue.Value.LeafValue, &e); err != nil {
			return false, err
		}
		glog.Infof("Got %#v", e)
		el := domains[e.Domain]
		pb.Merge(&el, &e)
		domains[e.Domain] = el
		glog.Infof("will update for %s", e.Domain)
	}
	glog.Infof("Got %d values, and %d proofs", len(getResp.KeyValue), proofs)

	glog.Info("Storing updated map values for domains...")
	// Store updated map values:
	setReq := &trillian.SetMapLeavesRequest{
		MapId:    m.mapID,
		KeyValue: make([]*trillian.KeyValue, 0, len(domains)),
	}
	for k, v := range domains {
		b, err := pb.Marshal(&v)
		if err != nil {
			return false, err
		}
		setReq.KeyValue = append(setReq.KeyValue, &trillian.KeyValue{Key: []byte(k), Value: &trillian.MapLeaf{
			LeafValue: b,
		}})
	}

	setReq.MapperData = meta

	setResp, err := m.vmap.SetLeaves(context.Background(), setReq)
	if err != nil {
		return false, err
	}
	glog.Infof("Set resp: %v", setResp)
	d := time.Now().Sub(start)
	glog.Infof("Map run complete, took %.1f secs to update %d values (%0.2f/s)", d.Seconds(), len(setReq.KeyValue), float64(len(setReq.KeyValue))/d.Seconds())
	return true, nil
}

func main() {
	flag.Parse()
	conn, err := grpc.Dial(*mapServer, grpc.WithInsecure())
	if err != nil {
		glog.Fatal(err)
	}
	defer conn.Close()

	mapper := CTMapper{
		mapID: int64(*mapID),
		ct:    client.New(*sourceLog, nil),
		vmap:  trillian.NewTrillianMapClient(conn),
	}

	for {
		moreToDo, err := mapper.oneMapperRun()
		if err != nil {
			glog.Warningf("mapper run failed: %v", err)
		}
		if !moreToDo {
			time.Sleep(5 * time.Second)
		}

	}
}
