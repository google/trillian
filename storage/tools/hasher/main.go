// Copyright 2017 Google LLC. All Rights Reserved.
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

// The hasher program provides a simple CLI for producing Merkle tree hashes. It is
// intended for use in development or debugging storage code.
package main

import (
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/golang/glog"
	"github.com/transparency-dev/merkle/rfc6962"
)

var base64Flag = flag.Bool("base64", false, "If true output in base64 instead of hex")

func decodeArgs(args []string) [][]byte {
	dec := make([][]byte, 0, len(args))

	for _, arg := range args {
		dh, err := hex.DecodeString(arg)
		if err != nil {
			glog.Fatalf("Input arg not a hex encoded string: %s: %v", arg, err)
		}
		dec = append(dec, dh)
	}

	return dec
}

func main() {
	flag.Parse()
	defer glog.Flush()

	decoded := decodeArgs(flag.Args())
	var hash []byte

	switch len(decoded) {
	case 1:
		// Leaf hash requested
		hash = rfc6962.DefaultHasher.HashLeaf(decoded[0])

	case 2:
		// Node hash requested
		hash = rfc6962.DefaultHasher.HashChildren(decoded[0], decoded[1])

	default:
		glog.Fatalf("Invalid number of arguments expected 1 (for leaf) or 2 (for node)")
	}

	if *base64Flag {
		fmt.Printf("%s\n", base64.StdEncoding.EncodeToString(hash))
	} else {
		fmt.Printf("%s\n", hex.EncodeToString(hash))
	}
}
