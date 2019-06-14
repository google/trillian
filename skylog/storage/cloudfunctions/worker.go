// Package cloudfunctions wraps Merkle tree build worker into a Cloud Function.
package cloudfunctions

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/spanner"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/skylog/core"
	cs "github.com/google/trillian/skylog/storage/cloudspanner"
)

const spannerEnvVar = "SPANNER_DB"

var (
	hasher  = rfc6962.DefaultHasher
	factory = &compact.RangeFactory{Hash: hasher.HashChildren}

	client *spanner.Client
)

// BuildJob is the payload of a Merkle tree building event.
type BuildJob struct {
	TreeID int64  `json:"tree_id"`
	Begin  uint64 `json:"begin"`
	End    uint64 `json:"end"`
}

// BuildSubtree consumes builder job message.
func BuildSubtree(ctx context.Context, job BuildJob) error {
	if job.End < job.Begin {
		return errors.New("invalid job: begin > end")
	}

	// TODO(pavelkalinnikov): Read hashes from storage.
	hashes := make([][]byte, 0, job.End-job.Begin)
	for i := job.Begin; i < job.End; i++ {
		data := []byte(fmt.Sprintf("data:%d", i))
		hash := hasher.HashLeaf(data)
		hashes = append(hashes, hash)
	}
	cJob := core.BuildJob{RangeStart: job.Begin, Hashes: hashes}

	opts := cs.TreeOpts{ShardLevels: 10, LeafShards: 16}
	ts := cs.NewTreeStorage(client, job.TreeID, opts)
	bw := core.NewBuildWorker(ts, factory)

	_, err := bw.Process(ctx, cJob)
	return err
}

func init() {
	ctx := context.Background()

	db, ok := os.LookupEnv(spannerEnvVar)
	if !ok {
		log.Fatalf("Environment variable %s not found", spannerEnvVar)
	}

	var err error
	if client, err = spanner.NewClient(ctx, db); err != nil {
		log.Fatalf("spanner.NewClient: %v", err)
	}
}
