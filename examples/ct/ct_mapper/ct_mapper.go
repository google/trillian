package main

import (
	"crypto/sha256"
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/client"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var sourceLog = flag.String("source", "https://ct.googleapis.com/testtube", "Source CT Log")
var mapServer = flag.String("map_server", "", "host:port for the map server")
var mapID = flag.Int("map_id", -1, "Map ID to write to")
var logBatchSize = flag.Int("log_batch_size", 256, "Max number of entries to process at a time from the CT Log")

//TODO(al): factor this out into a reusable thing.

type CTMapper struct {
	mapID int64
	ct    *client.LogClient
	vmap  trillian.TrillianMapClient
}

func updateDomainMap(m map[string]EntryList, domain string, index int64, isPrecert bool) {
	el := m[domain]
	el.Domain = domain
	if isPrecert {
		el.PrecertIndex = append(el.PrecertIndex, index)
	} else {
		el.CertIndex = append(el.CertIndex, index)
	}
	m[domain] = el
}

func (m *CTMapper) oneMapperRun() (bool, error) {
	glog.Info("starting mapping batch")
	getRootReq := &trillian.GetSignedMapRootRequest{m.mapID}
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
	domains := make(map[string]EntryList)
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
				return false, err
			}
			updateDomainMap(domains, cert.Subject.CommonName, entry.Index, false)
			for _, n := range cert.DNSNames {
				updateDomainMap(domains, n, entry.Index, false)
			}
		case ct.PrecertLogEntryType:
			precert, err := x509.ParseTBSCertificate(entry.Leaf.TimestampedEntry.PrecertEntry.TBSCertificate)
			if err != nil {
				return false, err
			}
			updateDomainMap(domains, precert.Subject.CommonName, entry.Index, true)
			for _, n := range precert.DNSNames {
				updateDomainMap(domains, n, entry.Index, true)
			}
		default:
			glog.Infof("Unknown logentry type at index %d", entry.Index)
			continue
		}
	}

	glog.Infof("Found %d unique domains from certs", len(domains))
	glog.Info("Fetching current map values for domains...")

	// Fetch the current map values for those domains:
	getReq := &trillian.GetMapLeavesRequest{
		MapId: m.mapID,
		Key:   make([][]byte, 0, len(domains)),
	}
	for k, _ := range domains {
		keyHash := sha256.Sum256([]byte(k))
		getReq.Key = append(getReq.Key, keyHash[:])
	}

	getResp, err := m.vmap.GetLeaves(context.Background(), getReq)
	if err != nil {
		return false, err
	}
	//glog.Info("Get resp: %v", getResp)

	for _, v := range getResp.KeyValue {
		e := EntryList{}
		if err := proto.Unmarshal(v.KeyValue.Value.LeafValue, &e); err != nil {
			return false, err
		}
		el := domains[e.Domain]
		proto.Merge(&el, &e)
		domains[e.Domain] = el
	}

	glog.Info("Storing updated map values for domains...")
	// Store updated map values:
	setReq := &trillian.SetMapLeavesRequest{
		MapId:    m.mapID,
		KeyValue: make([]*trillian.KeyValue, 0, len(domains)),
	}
	for k, v := range domains {
		keyHash := sha256.Sum256([]byte(k))
		b, err := proto.Marshal(&v)
		if err != nil {
			return false, err
		}
		setReq.KeyValue = append(setReq.KeyValue, &trillian.KeyValue{keyHash[:], &trillian.MapLeaf{
			LeafValue: b,
		}})
	}

	setReq.MapperData = meta

	setResp, err := m.vmap.SetLeaves(context.Background(), setReq)
	if err != nil {
		return false, err
	}
	glog.Infof("Set resp: %v", setResp)
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
