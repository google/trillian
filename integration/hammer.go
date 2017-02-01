package integration

import (
	"context"
	"crypto"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/client"
	"github.com/google/certificate-transparency/go/jsonclient"
	"github.com/google/certificate-transparency/go/merkletree"
	"github.com/google/certificate-transparency/go/tls"
	"github.com/google/certificate-transparency/go/x509"
	ctfe "github.com/google/trillian/examples/ct"
)

// How often to print stats.
const emitInterval = 1000

// How many STHs and SCTs to hold on to.
const sthCount = 10
const sctCount = 10

// Maximum number of entries to request.
const maxEntriesCount = uint64(10)

// HammerConfig provides configuration for a stress/load test.
type HammerConfig struct {
	// Configuration for the log.
	LogCfg ctfe.LogConfig
	// Maximum merge delay.
	MMD time.Duration
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

type submittedCert struct {
	leafData    []byte
	leafHash    [sha256.Size]byte
	sct         *ct.SignedCertificateTimestamp
	integrateBy time.Time
	precert     bool
}

// pendingCerts holds certificates that have been submitted that we want
// to check inclusion proofs for.  The array is ordered from oldest to
// most recent, but new entries are only appended when enough time has
// passed since the last append, so the SCTs that get checked are spread
// out across the MMD period.
type pendingCerts [sctCount]*submittedCert

func (pc *pendingCerts) canAppend(now time.Time, mmd time.Duration) bool {
	if pc[sctCount-1] != nil {
		return false // full already
	}
	if pc[0] == nil {
		return true // nothing yet
	}
	// Only allow append if enough time has passed, namely MMD/#savedSCTs.
	last := sctCount - 1
	for ; last >= 0; last-- {
		if pc[last] != nil {
			break
		}
	}
	lastTime := timeFromMS(pc[last].sct.Timestamp)
	nextTime := lastTime.Add(mmd / sctCount)
	return now.After(nextTime)
}

func (pc *pendingCerts) appendCert(submitted *submittedCert) {
	// Require caller to have checked canAppend().
	which := 0
	for ; which < sctCount; which++ {
		if pc[which] == nil {
			break
		}
	}
	pc[which] = submitted
}

// popIfMMDPassed returns the oldest submitted certificate (and removes it) if the
// maximum merge delay has passed, i.e. it is expected to be integrated as of now.
func (pc *pendingCerts) popIfMMDPassed(now time.Time) *submittedCert {
	if pc[0] == nil {
		return nil
	}
	submitted := pc[0]
	if !now.After(submitted.integrateBy) {
		// Oldest cert not due to be integrated yet, so neither will any others.
		return nil
	}
	// Can pop the oldest cert and shuffle the others along, which make room for
	// another cert to be stored.
	for i := 0; i < (sctCount - 1); i++ {
		pc[i] = pc[i+1]
	}
	pc[sctCount-1] = nil
	return submitted
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
	// STHs are arranged from later to earlier (so [0] is the most recent), and the
	// discovery of new STHs will push older ones off the end.
	sth := [sthCount]*ct.SignedTreeHead{}
	// Submitted certs also run from later to earlier, but the discovery of new SCTs
	// does not affect the existing contents of the array, so if the array is full it
	// keeps the same elements.  Instead, the oldest entry is removed (and a space
	// created) when we are able to get an inclusion proof for it.
	var pending pendingCerts

	for count := uint64(1); count < cfg.Operations; count++ {
		ep := cfg.EPBias.Choose()
		glog.V(3).Infof("perform %s operation", ep)
		status := http.StatusOK
		switch ep {
		case ctfe.AddChainName:
			chain, err := makeCertChain(cfg.LeafChain, cfg.LeafCert, cfg.CACert, cfg.Signer)
			if err != nil {
				return fmt.Errorf("failed to make fresh cert: %v", err)
			}
			sct, err := pool.Pick().AddChain(ctx, chain)
			if err != nil {
				return fmt.Errorf("failed to add-chain: %v", err)
			}
			glog.V(2).Infof("%s: Uploaded cert, got SCT(time=%q)", cfg.LogCfg.Prefix, timeFromMS(sct.Timestamp))
			if pending.canAppend(time.Now(), cfg.MMD) {
				// Calculate leaf hash =  SHA256(0x00 | tls-encode(MerkleTreeLeaf))
				submitted := submittedCert{precert: false, sct: sct}
				leaf := ct.MerkleTreeLeaf{
					Version:  ct.V1,
					LeafType: ct.TimestampedEntryLeafType,
					TimestampedEntry: &ct.TimestampedEntry{
						Timestamp:  sct.Timestamp,
						EntryType:  ct.X509LogEntryType,
						X509Entry:  &(chain[0]),
						Extensions: sct.Extensions,
					},
				}
				submitted.integrateBy = timeFromMS(sct.Timestamp).Add(cfg.MMD)
				submitted.leafData, err = tls.Marshal(leaf)
				if err != nil {
					return fmt.Errorf("failed to tls.Marshal leaf cert: %v", err)
				}
				submitted.leafHash = sha256.Sum256(append([]byte{merkletree.LeafPrefix}, submitted.leafData...))
				pending.appendCert(&submitted)
				glog.V(3).Infof("%s: Uploaded cert has leaf-hash %x", cfg.LogCfg.Prefix, submitted.leafHash)
			}
		case ctfe.AddPreChainName:
			prechain, tbs, err := makePrecertChain(cfg.LeafChain, cfg.LeafCert, cfg.CACert, cfg.Signer)
			if err != nil {
				return fmt.Errorf("failed to make fresh pre-cert: %v", err)
			}
			sct, err := pool.Pick().AddPreChain(ctx, prechain)
			if err != nil {
				return fmt.Errorf("failed to add-pre-chain: %v", err)
			}
			glog.V(2).Infof("%s: Uploaded pre-cert, got SCT(time=%q)", cfg.LogCfg.Prefix, timeFromMS(sct.Timestamp))
			if pending.canAppend(time.Now(), cfg.MMD) {
				// Calculate leaf hash =  SHA256(0x00 | tls-encode(MerkleTreeLeaf))
				submitted := submittedCert{precert: true, sct: sct}
				leaf := ct.MerkleTreeLeaf{
					Version:  ct.V1,
					LeafType: ct.TimestampedEntryLeafType,
					TimestampedEntry: &ct.TimestampedEntry{
						Timestamp: sct.Timestamp,
						EntryType: ct.PrecertLogEntryType,
						PrecertEntry: &ct.PreCert{
							IssuerKeyHash:  sha256.Sum256(cfg.CACert.RawSubjectPublicKeyInfo),
							TBSCertificate: tbs,
						},
						Extensions: sct.Extensions,
					},
				}
				submitted.integrateBy = timeFromMS(sct.Timestamp).Add(cfg.MMD)
				submitted.leafData, err = tls.Marshal(leaf)
				if err != nil {
					return fmt.Errorf("tls.Marshal(precertLeaf)=(nil,%v); want (_,nil)", err)
				}
				submitted.leafHash = sha256.Sum256(append([]byte{merkletree.LeafPrefix}, submitted.leafData...))
				pending.appendCert(&submitted)
				glog.V(3).Infof("%s: Uploaded pre-cert has leaf-hash %x", cfg.LogCfg.Prefix, submitted.leafHash)
			}
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
			glog.V(2).Infof("%s: Got STH(time=%q, size=%d)", cfg.LogCfg.Prefix, timeFromMS(sth[0].Timestamp), sth[0].TreeSize)
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
			if err := checkCTConsistencyProof(sth[which], sthNow, proof); err != nil {
				return fmt.Errorf("get-sth-consistency(%d, %d) proof check failed: %v", sth[which].TreeSize, sthNow.TreeSize, err)
			}
			glog.V(2).Infof("%s: Got STH consistency proof (size=%d => %d) len %d",
				cfg.LogCfg.Prefix, sth[which].TreeSize, sthNow.TreeSize, len(proof))
		case ctfe.GetProofByHashName:
			submitted := pending.popIfMMDPassed(time.Now())
			if submitted == nil {
				// No SCT that is guaranteed to be integrated, so move on.
				status = http.StatusFailedDependency
				break
			}
			// Get an STH that should include this submitted [pre-]cert.
			sth, err := pool.Pick().GetSTH(ctx)
			if err != nil {
				return fmt.Errorf("failed to get-sth for proof: %v", err)
			}
			// Get and check an inclusion proof.
			rsp, err := pool.Pick().GetProofByHash(ctx, submitted.leafHash[:], sth.TreeSize)
			if err != nil {
				return fmt.Errorf("failed to get-proof-by-hash(size=%d): %v", sth.TreeSize, err)
			}
			if err := Verifier.VerifyInclusionProof(rsp.LeafIndex, int64(sth.TreeSize), rsp.AuditPath, sth.SHA256RootHash[:], submitted.leafData); err != nil {
				return fmt.Errorf("failed to VerifyInclusionProof(%d, %d)=%v", rsp.LeafIndex, sth.TreeSize, err)
			}
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
