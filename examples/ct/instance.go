package ct

import (
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/util"
)

// LogConfig describes the configuration options for a log instance.
type LogConfig struct {
	LogID           int64
	Prefix          string
	RootsPEMFile    string
	PubKeyPEMFile   string
	PrivKeyPEMFile  string
	PrivKeyPassword string
}

var (
	logVars = expvar.NewMap("logs")
)

// LogStats matches the schema of the exported JSON stats for a particular log instance.
type LogStats struct {
	LogID            int                       `json:"log-id"`
	LastSCTTimestamp int                       `json:"last-sct-timestamp"`
	LastSTHTimestamp int                       `json:"last-sth-timestamp"`
	LastSTHTreesize  int                       `json:"last-sth-treesize"`
	HTTPAllReqs      int                       `json:"http-all-reqs"`
	HTTPAllRsps      map[string]int            `json:"http-all-rsps"` // status => count
	HTTPReq          map[string]int            `json:"http-reqs"`     // entrypoint => count
	HTTPRsps         map[string]map[string]int `json:"http-rsps"`     // entrypoint => status => count
}

// AllStats matches the schema of the entire exported JSON stats.
type AllStats struct {
	Logs map[string]LogStats `json:"logs"`
}

// LogConfigFromFile creates a slice of LogConfig options from the given
// filename, which should contain JSON encoded configuration data.
func LogConfigFromFile(filename string) ([]LogConfig, error) {
	if len(filename) == 0 {
		return nil, fmt.Errorf("log config filename empty")
	}
	cfgData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read log config: %v", err)
	}
	var cfg []LogConfig
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config data: %v", err)
	}
	if len(cfg) == 0 {
		return nil, errors.New("empty log config found")
	}
	return cfg, nil
}

// SetUpInstance sets up a log instance that uses the specified client to communicate
// with the Trillian RPC back end.
func (cfg LogConfig) SetUpInstance(client trillian.TrillianLogClient, deadline time.Duration) (*PathHandlers, error) {
	// Check config validity.
	if len(cfg.RootsPEMFile) == 0 {
		return nil, errors.New("need to specify RootsPEMFile")
	}
	if len(cfg.PubKeyPEMFile) == 0 {
		return nil, errors.New("need to specify PubKeyPEMFile")
	}
	if len(cfg.PrivKeyPEMFile) == 0 {
		return nil, errors.New("need to specify PrivKeyPEMFile")
	}

	// Load the trusted roots
	roots := NewPEMCertPool()
	if err := roots.AppendCertsFromPEMFile(cfg.RootsPEMFile); err != nil {
		return nil, fmt.Errorf("failed to read trusted roots: %v", err)
	}

	// Set up a key manager instance for this log.
	km := crypto.NewPEMKeyManager()
	privData, err := ioutil.ReadFile(cfg.PrivKeyPEMFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key file: %v", err)
	}

	if err := km.LoadPrivateKey(string(privData), cfg.PrivKeyPassword); err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	pubData, err := ioutil.ReadFile(cfg.PubKeyPEMFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key file: %v", err)
	}

	if err := km.LoadPublicKey(string(pubData)); err != nil {
		return nil, fmt.Errorf("failed to parse public key: %v", err)
	}

	// Create and register the handlers using the RPC client we just set up
	ctx := NewLogContext(cfg.LogID, cfg.Prefix, roots, client, km, deadline, new(util.SystemTimeSource))
	logVars.Set(cfg.Prefix, ctx.exp.vars)

	handlers := ctx.Handlers(cfg.Prefix)
	return &handlers, nil
}
