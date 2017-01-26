package integration

import (
	"context"
	"crypto"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang/glog"
	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/client"
	"github.com/google/certificate-transparency/go/jsonclient"
	"github.com/google/certificate-transparency/go/x509"
	ctfe "github.com/google/trillian/examples/ct"
)

// How often to print stats.
const emitInterval = 1000

// How many STHs to hold on to.
const sthCount = 10

// Maximum number of entries to request.
const maxEntriesCount = uint64(10)

// HammerConfig provides configuration for a stress/load test.
type HammerConfig struct {
	// Configuration for the log.
	LogCfg ctfe.LogConfig
	// Leaf certificate chain to use as template.
	LeafChain []ct.ASN1Cert
	// Parsed leaf certificate to use as template.
	LeafCert *x509.Certificate
	// Intermediate CA certificate chain to use as re-signing CA.
	CACert *x509.Certificate
	Signer crypto.Signer
	// Comma-separated list of servers providing the log
	Servers string
	// Bias values to favor particular log operations
	EPBias HammerBias
	// Number of operations to perform.
	Operations uint64
}

// HammerBias indicates the bias for selecting different log operations.
type HammerBias struct {
	Bias  map[ctfe.EntrypointName]int
	total int
}

// Choose randomly picks an operation to perform according to the biases.
func (hb HammerBias) Choose() ctfe.EntrypointName {
	if hb.total == 0 {
		for _, ep := range ctfe.Entrypoints {
			hb.total += hb.Bias[ep]
		}
	}
	which := rand.Intn(hb.total)
	for _, ep := range ctfe.Entrypoints {
		which -= hb.Bias[ep]
		if which < 0 {
			return ep
		}
	}
	panic("random choice out of range")
}

// HammerCTLog performs load/stress operations according to given config.
func HammerCTLog(cfg HammerConfig) error {
	opts := jsonclient.Options{}
	if cfg.LogCfg.PubKeyPEMFile != "" {
		pubkey, err := ioutil.ReadFile(cfg.LogCfg.PubKeyPEMFile)
		if err != nil {
			return fmt.Errorf("failed to get public key contents: %v", err)
		}
		opts.PublicKey = string(pubkey)
	}
	var pool ClientPool
	for _, s := range strings.Split(cfg.Servers, ",") {
		c, err := client.New("http://"+s+"/"+cfg.LogCfg.Prefix, nil, opts)
		if err != nil {
			return fmt.Errorf("failed to create LogClient instance: %v", err)
		}
		pool = append(pool, c)
	}
	ctx := context.Background()

	stats := newWantStats(cfg.LogCfg.LogID)
	sth := [sthCount]*ct.SignedTreeHead{}
	for count := uint64(1); count < cfg.Operations; count++ {
		ep := cfg.EPBias.Choose()
		glog.V(3).Infof("perform %s operation", ep)
		status := http.StatusOK
		switch ep {
		case ctfe.AddChainName:
			chain, err := MakeCertChain(cfg.LeafChain, cfg.LeafCert, cfg.CACert, cfg.Signer)
			if err != nil {
				return fmt.Errorf("failed to make fresh cert: %v", err)
			}
			sct, err := pool.Pick().AddChain(ctx, chain)
			if err != nil {
				return fmt.Errorf("failed to add-chain: %v", err)
			}
			glog.V(2).Infof("%s: Uploaded cert, got SCT(time=%q)", cfg.LogCfg.Prefix, TimeFromMS(sct.Timestamp))
		case ctfe.AddPreChainName:
			prechain, _, err := MakePrecertChain(cfg.LeafChain, cfg.LeafCert, cfg.CACert, cfg.Signer)
			if err != nil {
				return fmt.Errorf("failed to make fresh pre-cert: %v", err)
			}
			sct, err := pool.Pick().AddPreChain(ctx, prechain)
			if err != nil {
				return fmt.Errorf("failed to add-pre-chain: %v", err)
			}
			glog.V(2).Infof("%s: Uploaded pre-cert, got SCT(time=%q)", cfg.LogCfg.Prefix, TimeFromMS(sct.Timestamp))
		case ctfe.GetSTHName:
			// Shuffle earlier STHs along.
			for i := sthCount - 1; i > 0; i-- {
				sth[i] = sth[i-1]
			}
			var err error
			sth[0], err = pool.Pick().GetSTH(ctx)
			if err != nil {
				return fmt.Errorf("failed to get-sth: %v", err)
			}
			glog.V(2).Infof("%s: Got STH(time=%q, size=%d)", cfg.LogCfg.Prefix, TimeFromMS(sth[0].Timestamp), sth[0].TreeSize)
		case ctfe.GetSTHConsistencyName:
			// Get current size, and pick an earlier size
			sthNow, err := pool.Pick().GetSTH(ctx)
			if err != nil {
				return fmt.Errorf("failed to get-sth for current tree: %v", err)
			}
			which := rand.Intn(sthCount)
			if sth[which] == nil || sth[which].TreeSize == 0 {
				glog.V(3).Infof("%s: skipping get-sth-consistency as no earlier STH", cfg.LogCfg.Prefix)
				break
			}
			if sth[which].TreeSize == sthNow.TreeSize {
				glog.V(3).Infof("%s: skipping get-sth-consistency as same size (%d)", cfg.LogCfg.Prefix, sthNow.TreeSize)
				break
			}

			proof, err := pool.Pick().GetSTHConsistency(ctx, sth[which].TreeSize, sthNow.TreeSize)
			if err != nil {
				return fmt.Errorf("failed to get-sth-consistency(%d, %d): %v", sth[which].TreeSize, sthNow.TreeSize, err)
			}
			if err := CheckCTConsistencyProof(sth[which], sthNow, proof); err != nil {
				return fmt.Errorf("get-sth-consistency(%d, %d) proof check failed: %v", sth[which].TreeSize, sthNow.TreeSize, err)
			}
			glog.V(2).Infof("%s: Got STH consistency proof (size=%d => %d) len %d",
				cfg.LogCfg.Prefix, sth[which].TreeSize, sthNow.TreeSize, len(proof))
		case ctfe.GetProofByHashName:
			// TODO(drysdale): figure out a way to check inclusion proofs, when enough time has passed for inclusion
			status = http.StatusNotImplemented
		case ctfe.GetEntriesName:
			if sth[0] == nil || sth[0].TreeSize == 0 {
				glog.V(3).Infof("%s: skipping get-entries as no earlier STH", cfg.LogCfg.Prefix)
				break
			}
			count := uint64(1 + rand.Intn(int(maxEntriesCount-1)))
			if count > sth[0].TreeSize {
				count = sth[0].TreeSize
			}
			// Entry indices are zero-based.
			first := int64(sth[0].TreeSize - count)
			last := int64(sth[0].TreeSize) - 1
			entries, err := pool.Pick().GetEntries(ctx, first, last)
			if err != nil {
				return fmt.Errorf("failed to get-entries(%d,%d): %v", first, last, err)
			}
			if len(entries) < int(count) {
				return fmt.Errorf("get-entries(%d,%d) returned %d entries; want %d", first, last, len(entries), count)
			}
			for i, entry := range entries {
				leaf := entry.Leaf
				ts := leaf.TimestampedEntry
				if leaf.Version != 0 {
					return fmt.Errorf("leaf[%d].Version=%v; want V1(0)", i, leaf.Version)
				}
				if leaf.LeafType != ct.TimestampedEntryLeafType {
					return fmt.Errorf("leaf[%d].Version=%v; want TimestampedEntryLeafType", i, leaf.LeafType)
				}
				if ts.EntryType != ct.X509LogEntryType && ts.EntryType != ct.PrecertLogEntryType {
					return fmt.Errorf("leaf[%d].ts.EntryType=%v; want {X509,Precert}LogEntryType", i, ts.EntryType)
				}
			}
			glog.V(2).Infof("%s: Got entries [%d:%d+1]\n", cfg.LogCfg.Prefix, first, last)
		case ctfe.GetRootsName:
			roots, err := pool.Pick().GetAcceptedRoots(ctx)
			if err != nil {
				return fmt.Errorf("failed to get-roots: %v", err)
			}
			glog.V(2).Infof("%s: Got roots (len=%d)", cfg.LogCfg.Prefix, len(roots))
		case ctfe.GetEntryAndProofName:
			status = http.StatusNotImplemented
		default:
			return fmt.Errorf("internal error: unknown entrypoint %s selected", ep)
		}
		if status == http.StatusNotImplemented {
			glog.V(2).Infof("%s: hammering entrypoint %s not yet implemented", cfg.LogCfg.Prefix, ep)
		}
		stats.done(ep, status)

		if count%emitInterval == 0 {
			fmt.Printf("%10s:", cfg.LogCfg.Prefix)
			fmt.Printf(" last-sth.size=%d", sth[0].TreeSize)
			fmt.Printf(" operations: total=%d", count)
			statusOK := strconv.Itoa(http.StatusOK)
			for _, ep := range ctfe.Entrypoints {
				if cfg.EPBias.Bias[ep] > 0 {
					fmt.Printf(" %s=%d/%d", ep, stats.HTTPRsps[ep][statusOK], stats.HTTPReq[ep])
				}
			}
			fmt.Printf("\n")
		}
	}

	return nil
}
