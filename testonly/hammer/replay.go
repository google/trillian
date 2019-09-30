// Copyright 2018 Google Inc. All Rights Reserved.
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

package hammer

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"google.golang.org/grpc"
)

type recordingInterceptor struct {
	mu     sync.Mutex
	outLog io.Writer
}

// NewRecordingInterceptor returns a grpc.UnaryClientInterceptor that logs outgoing
// requests to file.
func NewRecordingInterceptor(filename string) (grpc.UnaryClientInterceptor, error) {
	o, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %v", err)
	}
	ri := recordingInterceptor{outLog: o}
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return ri.invoke(ctx, method, req, reply, cc, invoker, opts...)
	}, nil
}

func (ri *recordingInterceptor) invoke(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if msg, ok := req.(proto.Message); ok {
		ri.dumpMessage(msg)
	} else {
		glog.Warningf("failed to convert request %T to proto.Message", req)
	}
	err := invoker(ctx, method, req, reply, cc, opts...)
	if err == nil {
		if msg, ok := reply.(proto.Message); ok {
			ri.dumpMessage(msg)
		} else {
			glog.Warningf("failed to convert response %T to proto.Message", req)
		}
	}
	return err
}

func (ri *recordingInterceptor) dumpMessage(in proto.Message) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	if err := writeMessage(ri.outLog, in); err != nil {
		glog.Error(err.Error())
	}
}

func writeMessage(w io.Writer, in proto.Message) error {
	a, err := ptypes.MarshalAny(in)
	if err != nil {
		return fmt.Errorf("failed to marshal %T %+v to any.Any: %v", in, in, err)
	}
	data, err := proto.Marshal(a)
	if err != nil {
		return fmt.Errorf("failed to marshal any.Any: %v", err)
	}
	// Encode as [4-byte big-endian length, message]
	lenData := make([]byte, 4)
	binary.BigEndian.PutUint32(lenData, uint32(len(data)))
	if _, err := w.Write(lenData); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return nil
}

func readMessage(r io.Reader) (*any.Any, error) {
	// Decode from [4-byte big-endian length, message]
	var l uint32
	if err := binary.Read(r, binary.BigEndian, &l); err != nil {
		if err != io.EOF {
			err = fmt.Errorf("corrupt data: expected 4-byte length: %v", err)
		}
		return nil, err
	}
	data := make([]byte, l)
	n, err := r.Read(data)
	if uint32(n) < l {
		return nil, fmt.Errorf("corrupt data: expected %d bytes of data, only found %d bytes", n, l)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %d bytes of data: %v", l, err)
	}
	var a any.Any
	if err := proto.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("failed to unmarshal into any.Any: %v", err)
	}
	return &a, nil
}

// ReplayFile reads recorded gRPC requests and re-issues them using the given
// client.  If a request has a MapId field, and its value is present in mapmap,
// then the MapId field is replaced before replay.
func ReplayFile(ctx context.Context, r io.Reader, cl trillian.TrillianMapClient, write trillian.TrillianMapWriteClient, mapmap map[int64]int64) error {
	for {
		a, err := readMessage(r)
		if err != nil {
			if err != io.EOF {
				glog.Errorf("Error reading message: %v", err)
				return err
			}
			// We hit EOF - expected.
			return nil
		}
		glog.V(2).Infof("Replay %q", a.TypeUrl)
		if err := replayMessage(ctx, cl, write, a, mapmap); err != nil {
			return err
		}
	}
}

// convertMessage modifies msg in-place so that the contents of a "MapId" field
// are updated according to mapmap.
func convertMessage(msg proto.Message, mapmap map[int64]int64) {
	// Look for a "MapId" field that we can overwrite if needed.
	pVal := reflect.ValueOf(msg)
	if fieldVal := pVal.Elem().FieldByName("MapId"); fieldVal.CanSet() {
		from := fieldVal.Int()
		if to, ok := mapmap[from]; ok {
			glog.V(2).Infof("Replacing msg.MapId=%d with %d in %T", from, to, msg)
			fieldVal.SetInt(to)
		}
	}
}

func replayMessage(ctx context.Context, cl trillian.TrillianMapClient, write trillian.TrillianMapWriteClient, a *any.Any, mapmap map[int64]int64) error {
	var da ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(a, &da); err != nil {
		return fmt.Errorf("failed to unmarshal from any.Any: %v", err)
	}
	req := da.Message
	convertMessage(req, mapmap)
	glog.V(2).Infof("Request req=%T %+v", req, req)
	var err error
	var rsp proto.Message
	if cl != nil {
		switch req := req.(type) {
		case *trillian.GetMapLeavesRequest:
			rsp, err = cl.GetLeaves(ctx, req)
		case *trillian.GetMapLeavesByRevisionRequest:
			rsp, err = cl.GetLeavesByRevision(ctx, req)
		case *trillian.WriteMapLeavesRequest:
			rsp, err = write.WriteLeaves(ctx, req)
		case *trillian.GetSignedMapRootRequest:
			rsp, err = cl.GetSignedMapRoot(ctx, req)
		case *trillian.GetSignedMapRootByRevisionRequest:
			rsp, err = cl.GetSignedMapRootByRevision(ctx, req)
		case *trillian.InitMapRequest:
			rsp, err = cl.InitMap(ctx, req)
		}
		if rsp != nil {
			glog.V(1).Infof("Request:  req=%T %+v", req, req)
			glog.V(1).Infof("Response: rsp=%T %+v err=%v", rsp, rsp, err)
		}
	}
	return err
}
