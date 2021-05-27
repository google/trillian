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

// Package keys provides access to public and private keys for signing and verification of signatures.
package keys

import (
	"context"
	"crypto"
	"fmt"
	"sync"

	"github.com/golang/glog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ProtoHandler uses the information in a protobuf message to obtain a crypto.Signer.
// For example, the protobuf message may contain a key or identify where a key can be found.
type ProtoHandler func(context.Context, proto.Message) (crypto.Signer, error)

var (
	// handlers convert a protobuf message into a crypto.Signer.
	handlers   = make(map[protoreflect.FullName]ProtoHandler)
	handlersMu sync.RWMutex
)

// RegisterHandler enables transformation of protobuf messages of the same
// type as keyProto into crypto.Signer by invoking the provided handler.
// The keyProto need only be an empty example of the type of protobuf message that
// the handler can process - only its type is examined.
// If a handler for this type of protobuf message has already been added, it will
// be replaced.
func RegisterHandler(keyProto proto.Message, handler ProtoHandler) {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	keyProtoType := keyProto.ProtoReflect().Descriptor().FullName()

	if _, alreadyExists := handlers[keyProtoType]; alreadyExists {
		glog.Warningf("Overridding ProtoHandler for protobuf %q", keyProtoType)
	}

	handlers[keyProtoType] = handler
}

// unregisterHandler removes a previously-added protobuf message handler.
// See RegisterHandler().
func unregisterHandler(keyProto proto.Message) {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	delete(handlers, keyProto.ProtoReflect().Descriptor().FullName())
}

// NewSigner uses a registered ProtoHandler (see RegisterHandler()) to convert a
// protobuf message into a crypto.Signer.
// If there is no ProtoHandler registered for this type of protobuf message, an
// error will be returned.
func NewSigner(ctx context.Context, keyProto proto.Message) (crypto.Signer, error) {
	handlersMu.RLock()
	defer handlersMu.RUnlock()
	if keyProto == nil {
		return nil, fmt.Errorf("nil keyProto")
	}
	keyProtoType := keyProto.ProtoReflect().Descriptor().FullName()

	if handler, ok := handlers[keyProtoType]; ok {
		return handler(ctx, keyProto)
	}

	return nil, fmt.Errorf("no ProtoHandler registered for protobuf %q", keyProtoType)
}
