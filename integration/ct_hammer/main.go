package main

import (
	"flag"
	"fmt"
	"math/rand"
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
		glog.Fatal("Test aborted as no log config provided")
	}
	if *seed == -1 {
		*seed = time.Now().UTC().UnixNano() & 0xFFFFFFFF
	}
	rand.Seed(*seed)

	cfg, err := ctfe.LogConfigFromFile(*logConfigFlag)
	if err != nil {
		glog.Fatalf("Failed to read log config: %v", err)
	}

	// Retrieve the test data.
	caChain, err := integration.GetChain(*testDir, "int-ca.cert")
	if err != nil {
		glog.Fatalf("failed to load certificate: %v", err)
	}
	leafChain, err := integration.GetChain(*testDir, "leaf01.chain")
	if err != nil {
		glog.Fatalf("failed to load certificate: %v", err)
	}
	signer, err := integration.MakeSigner(*testDir)
	if err != nil {
		glog.Fatalf("failed to retrieve signer for re-signing: %v", err)
	}
	leafCert, err := x509.ParseCertificate(leafChain[0].Data)
	if err != nil {
		glog.Fatalf("failed to parse leaf certificate to build precert from: %v", err)
	}
	caCert, err := x509.ParseCertificate(caChain[0].Data)
	if err != nil {
		glog.Fatalf("failed to parse issuer for precert: %v", err)
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

	fmt.Print(`
                                   ':++/.
                                  'sddhhyo'
                                  /hyyho+s-       '-:/+:.
                                 .sdhhysoy-  ' '/sdmNNmmy'
                             ':oooymmmdddmysymhmNNNNNNNh-
                      '.:::+so++++ymmmNNNdyyyNNNNNMMNd/'
             '...:::/://osoo++s+yyhmNNNMmdddhyymNNhs+.
     '..-://+++/////+//+sooosyyhdmmdNNNMmmmhhs+y/-'
    'oooooooo++++++/ossyyyyhhhhdddyymNNmhdmmdy:-'
  ':ohhso++/++//+/+/////:oyyyddhhy+/hmNNNMMMmo-'
  -hddo-''               +syyhhyyy+:ymNNMMNms:'
  'ss+'                  /sssyssyyo/sdmmmds+/.
   ''                    +sssssyyyysyhhyys+:.'
                         +ssyyssoosoosss+/:.'
                        -yyyyysooso+so+/::-'
                        smmdhyssssoso+//:-'
                       -mNMMMNdyssyso+/:.'
                   ':shmMMMMMMMMNMMNmo.
                  -hNMMMMMMMMMMMMMMMMd/'
                 .hNMMMMMMMMMMMMMMMMMMNy/.
                .yNMMMMMMMMMMMMMMMMMMMMMMd:
              .omMMMMMMMMMMMMMMMMMMMMNMMMMNo'
            .omMMMMMMMMMMMMMMMMMMMMMMMMMMMMMy.
           -dNMMMMMMMMMMMMMMMMMNNMMMMMNMMMMMMy'
          :dMMMMMMMMMMMMMMMMMMMNmNMMMMNNMMMMMN/
         .dMMMMMMMMMMMMMMMMMMMNmmMMMMMMMMMMMMMs'
         +NMMMMMMMMMMMMNmMMMMMNmNMMMMMMMMMMMMMy'
         -mMMMMMMMMMMMMMMMMMMMNNMMMMMMMMMMMMMMo'
         'hNMMMMMMMMMMMMMMMMMMMMMMMMMMMNNNMNNh.
          sNMMMMMMMMMMMMMMMMMMMMMMMMMMNNNNmh+.
          /NMMMMMMMMMMMMMMMMMNdmNMMMMMNNNm:
          -mMMMMMMMMMMMMMMNms-''/mMMMNddNm-
           oNMMMMMMMMMMMMd/'     +NMMNddmm:
           -mMMMMMMMMMMMN+'      'sNMMMNNmy-
            sNMMMMMMMMMNd.        'sNNMNNNmh-
            :NMMMMMMMMNm:          'yNNMMNNmy'
            'yNMMMMMMMNo'           .hNMMMNNm.
             .dMMMMMNNy'             -dNMMNNN:
              oNMMMNNm-               /NNMMNN/
              :NMMMMNh.                yNMMNms'
              :mMMMNN/                 -mMMMNy'
              'yNMmho'                  sNNmNNs.
            ''/mMMMmy'                  -mNMMMMNdhhy+'
          .yNNMMMMMNm+'                 /NMMMNNNmdy+-
           :hNNMMMNdd/'                 /dmNNdso+:.'`)
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
	for e := range results {
		if e.err != nil {
			glog.Errorf("%s: %v", e.prefix, e.err)
		}
	}
}
