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

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	spb "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/testonly"

	_ "github.com/go-sql-driver/mysql"
)

var mySQLURIFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "")

func main() {
	flag.Parse()
	glog.Info("Starting...")

	db, err := mysql.OpenDB(*mySQLURIFlag)
	if err != nil {
		glog.Exitf("Failed to open DB connection: %v", err)
	}

	mapID := int64(1)
	ms := mysql.NewMapStorage(db)
	hasher := maphasher.Default

	testVecs := []struct {
		batchSize       int
		numBatches      int
		expectedRootB64 string
	}{
		// roots calculated using python code.
		{1024, 4, "Av30xkERsepT6F/AgbZX3sp91TUmV1TKaXE6QPFfUZA="},
		{10, 4, "6Pk5sprCr3ACfo0OLRZw7sAGdIBTc+7+MxfdW3n76Pc="},
		{6, 4, "QZJ42Te4bw+uGdUaIqzhelxpERU5Ru6uLdy0ixJAuWQ="},
		{4, 4, "9myL1k8Ik6m3Q3JXljHLzfNQHS2d5X6CCbpE/x3mixg="},
		{5, 4, "4xyGOe2DQYi2Qb4aBto9R7jSmiRYqfJ+TcMxUZTXMkM="},
		{6, 3, "FeB/9D+Gzo6oYB2Zi2JMHdrr9KvfvMk7o6DOzjPYG4w="},
		{10, 3, "RfJ6JPERbkDiwlov8/alCqr4yeYYIWl3dWWS3trHsiY="},
		{1, 4, "pQhTahkoXM3WTeAO1o8BYKhgMNzS1yG03vg/fQSVyIc="},
		{2, 4, "RdcEkg5qEuW5eV3VJJLr6uSzvlc27D55AZChG76LHGA="},
		{4, 1, "3dpnVw5Le3HDq/GAkGoSYT9VkzJRV8z18huOk5qMbco="},
		{1024, 1, "7R5uvGy5MJ2Y8xrQr4/mnn3aPw39vYscghmg9KBJaKc="},
		{1, 2, "cZIYiv7ZQ/3rBfpCrha1NKdUnQ8NsTm21WWdV3P4qcU="},
		{1, 3, "KUaQinjLtPQ/ZAek4nHrR7tVXDxLt5QsvZK3vGopDkA="}}

	const testIndex = 0

	batchSize := testVecs[testIndex].batchSize
	numBatches := testVecs[testIndex].numBatches
	expectedRootB64 := testVecs[testIndex].expectedRootB64

	ctx := context.Background()

	var root []byte
	for x := 0; x < numBatches; x++ {
		tx, err := ms.BeginForTree(ctx, mapID)
		if err != nil {
			glog.Exitf("Failed to Begin() a new tx: %v", err)
		}
		defer tx.Close()
		w, err := merkle.NewSparseMerkleTreeWriter(
			ctx,
			mapID,
			tx.WriteRevision(),
			hasher,
			func() (storage.TreeTX, error) {
				return ms.BeginForTree(ctx, mapID)
			})
		if err != nil {
			glog.Exitf("Failed to create new SMTWriter: %v", err)
		}

		glog.Infof("Starting batch %d...", x)
		h := make([]merkle.HashKeyValue, batchSize)
		for y := 0; y < batchSize; y++ {
			index := testonly.HashKey(fmt.Sprintf("key-%d-%d", x, y))
			leafHash, err := hasher.HashLeaf(mapID, index, []byte(fmt.Sprintf("value-%d-%d", x, y)))
			if err != nil {
				glog.Exitf("HashLeaf(): %v", err)
			}
			h[y].HashedKey = index
			h[y].HashedValue = leafHash
		}
		glog.Infof("Created %d k/v pairs...", len(h))

		glog.Info("SetLeaves...")
		if err := w.SetLeaves(ctx, h); err != nil {
			glog.Exitf("Failed to batch %d: %v", x, err)
		}
		glog.Info("SetLeaves done.")

		glog.Info("CalculateRoot...")
		root, err = w.CalculateRoot()
		if err != nil {
			glog.Exitf("Failed to calculate root hash: %v", err)
		}
		glog.Infof("CalculateRoot (%d), root: %s", x, base64.StdEncoding.EncodeToString(root))

		if err := tx.StoreSignedMapRoot(ctx, trillian.SignedMapRoot{
			TimestampNanos: time.Now().UnixNano(),
			RootHash:       root,
			MapId:          mapID,
			MapRevision:    tx.WriteRevision(),
			Signature:      &spb.DigitallySigned{},
		}); err != nil {
			glog.Exitf("Failed to store SMR: %v", err)
		}

		if err := tx.Commit(); err != nil {
			glog.Exitf("Failed to Commit() tx: %v", err)
		}
	}

	if expected, got := testonly.MustDecodeBase64(expectedRootB64), root; !bytes.Equal(expected, root) {
		glog.Exitf("Expected root %s, got root: %s", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
	}
	glog.Infof("Finished, root: %s", base64.StdEncoding.EncodeToString(root))

}
