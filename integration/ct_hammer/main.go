// Copyright 2017 Google Inc. All Rights Reserved.
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

// ct_hammer is a stress/load test for a CT log.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/certificate-transparency/go/x509"
	ctfe "github.com/google/trillian/examples/ct"
	"github.com/google/trillian/integration"
)

var (
	httpServersFlag = flag.String("ct_http_servers", "localhost:8092", "Comma-separated list of (assumed interchangeable) servers, each as address:port")
	testDir         = flag.String("testdata_dir", "testdata", "Name of directory with test data")
	seed            = flag.Int64("seed", -1, "Seed for random number generation")
	logConfigFlag   = flag.String("log_config", "", "File holding log config in JSON")
	mmdFlag         = flag.Duration("mmd", 2*time.Minute, "MMD for logs")
	operationsFlag  = flag.Uint64("operations", ^uint64(0), "Number of operations to perform")
)
var (
	addChainBias          = flag.Int("add_chain", 20, "Bias for add-chain operations")
	addPreChainBias       = flag.Int("add_pre_chain", 20, "Bias for add-pre-chain operations")
	getSTHBias            = flag.Int("get_sth", 2, "Bias for get-sth operations")
	getSTHConsistencyBias = flag.Int("get_sth_consistency", 2, "Bias for get-sth-consistency operations")
	getProofByHashBias    = flag.Int("get_proof_by_hash", 2, "Bias for get-proof-by-hash operations")
	getEntriesBias        = flag.Int("get_entries", 2, "Bias for get-entries operations")
	getRootsBias          = flag.Int("get_roots", 1, "Bias for get-roots operations")
	getEntryAndProofBias  = flag.Int("get_entry_and_proof", 0, "Bias for get-entry-and-proof operations")
)

func main() {
	flag.Parse()
	if *logConfigFlag == "" {
		glog.Exit("Test aborted as no log config provided (via --log_config)")
	}
	if *seed == -1 {
		*seed = time.Now().UTC().UnixNano() & 0xFFFFFFFF
	}
	fmt.Printf("Today's test has been brought to you by the letters C and T and the number %#x\n", *seed)
	rand.Seed(*seed)

	cfg, err := ctfe.LogConfigFromFile(*logConfigFlag)
	if err != nil {
		glog.Exitf("Failed to read log config: %v", err)
	}

	// Retrieve the test data.
	caChain, err := integration.GetChain(*testDir, "int-ca.cert")
	if err != nil {
		glog.Exitf("failed to load certificate: %v", err)
	}
	leafChain, err := integration.GetChain(*testDir, "leaf01.chain")
	if err != nil {
		glog.Exitf("failed to load certificate: %v", err)
	}
	signer, err := integration.MakeSigner(*testDir)
	if err != nil {
		glog.Exitf("failed to retrieve signer for re-signing: %v", err)
	}
	leafCert, err := x509.ParseCertificate(leafChain[0].Data)
	if err != nil {
		glog.Exitf("failed to parse leaf certificate to build precert from: %v", err)
	}
	caCert, err := x509.ParseCertificate(caChain[0].Data)
	if err != nil {
		glog.Exitf("failed to parse issuer for precert: %v", err)
	}
	bias := integration.HammerBias{Bias: map[ctfe.EntrypointName]int{
		ctfe.AddChainName:          *addChainBias,
		ctfe.AddPreChainName:       *addPreChainBias,
		ctfe.GetSTHName:            *getSTHBias,
		ctfe.GetSTHConsistencyName: *getSTHConsistencyBias,
		ctfe.GetProofByHashName:    *getProofByHashBias,
		ctfe.GetEntriesName:        *getEntriesBias,
		ctfe.GetRootsName:          *getRootsBias,
		ctfe.GetEntryAndProofName:  *getEntryAndProofBias,
	}}

	fmt.Print("\n\nStop")
	for i := 0; i < 8; i++ {
		time.Sleep(100 * time.Millisecond)
		fmt.Print(".")
	}
	mc := "H4sIAAAAAAAA/4xVPbLzMAjsv1OkU8FI9LqDOAUFDUNBxe2/QXYSS/HLe5SeXZYfsf73+D1KB8D2B2RxZpGw8gcsSoQYeH1ya0fof1BpnhpuUR+P8ijorESq8Yto6WYWqsrMGh4qSkdI/YFZWu8d3AAAkklEHBGTNAYxbpKltWRgRzQ3A3CImDIjVSVCicThbLK0VjsiAGAGIIKbmUcIq/KkqYo4BNZDqtgZMAPNPSJCRISZZ36d5OiTUbqJZAOYIoCHUreImJsCPMobQ20SqjBbLWWbBGRREhHQU2MMUu9TwB12cC7X3SNrs1yPKvv5gD4yn+kzshOfMg69fVknJNbdcsjuDvgNXWPmTXCuEnuvP4NdlSWymIQjfsFWzbERZ5sz730NpbvoOGMOzu7eeBUaW3w8r4z2iRuD4uY6W9wgZ96+YZvpHW7SabvlH7CviKWQyp81EL2zj7Fcbee7MpSuNHzj2z18LdAvAkAr8pr/3cGFUO+apa2n64TK3XouTBpEch2Rf8GnzajAFY438+SzgURfV7sXT+q1FNTJYdLF9WxJzFheAyNmXfKuiel5/mW2QqSx2umlQ+L2GpTPWZBu5tvpXW5/fy4xTYd2ly+vR052dZbjTIh0u4vzyRDF6kPzoRLRfhp2pqnr5wce5eAGP6onaRv8EYdl7gfd5zIId/gxYvr4pWW7KnbjoU6kRL62e25b44ZQz7Oaf4GrTovnqemNsyOdL40Dls11ocMPn29nYeUvmt3S1v8DAAD//wEAAP//TRo+KHEIAAA="
	mcData, _ := base64.StdEncoding.DecodeString(mc)
	b := bytes.NewReader(mcData)
	r, _ := gzip.NewReader(b)
	io.Copy(os.Stdout, r)
	r.Close()
	fmt.Print("\n\nHammer Time\n\n")

	type result struct {
		prefix string
		err    error
	}
	results := make(chan result, len(cfg))
	var wg sync.WaitGroup
	for _, c := range cfg {
		wg.Add(1)
		cfg := integration.HammerConfig{
			LogCfg:     c,
			MMD:        *mmdFlag,
			LeafChain:  leafChain,
			LeafCert:   leafCert,
			CACert:     caCert,
			Signer:     signer,
			Servers:    *httpServersFlag,
			EPBias:     bias,
			Operations: *operationsFlag,
		}
		go func(cfg integration.HammerConfig) {
			defer wg.Done()
			err := integration.HammerCTLog(cfg)
			results <- result{prefix: cfg.LogCfg.Prefix, err: err}
		}(cfg)
	}
	wg.Wait()
	close(results)
	errCount := 0
	for e := range results {
		if e.err != nil {
			errCount++
			glog.Errorf("%s: %v", e.prefix, e.err)
		}
	}
	if errCount > 0 {
		os.Exit(1)
	}
}
