// Package registry provides a mechanism to register and discover tree hashers.
package registry

import (
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
)

var (
	logHashers = make(map[trillian.HashStrategy]hashers.LogHasher)
	mapHashers = make(map[trillian.HashStrategy]hashers.MapHasher)
)

// RegisterLogHasher registers a hasher for use.
func RegisterLogHasher(h trillian.HashStrategy, f hashers.LogHasher) {
	if h == trillian.HashStrategy_UNKNOWN_HASH_STRATEGY {
		panic(fmt.Sprintf("RegisterLogHasher(%s) of unknown hasher", h))
	}
	if logHashers[h] != nil {
		panic(fmt.Sprintf("%v already registered as a LogHasher", h))
	}
	logHashers[h] = f
}

// RegisterMapHasher registers a hasher for use.
func RegisterMapHasher(h trillian.HashStrategy, f hashers.MapHasher) {
	if h == trillian.HashStrategy_UNKNOWN_HASH_STRATEGY {
		panic(fmt.Sprintf("RegisterMapHasher(%s) of unknown hasher", h))
	}
	if mapHashers[h] != nil {
		panic(fmt.Sprintf("%v already registered as a MapHasher", h))
	}
	mapHashers[h] = f
}

// NewLogHasher returns a LogHasher.
func NewLogHasher(h trillian.HashStrategy) (hashers.LogHasher, error) {
	f := logHashers[h]
	if f != nil {
		return f, nil
	}
	return nil, fmt.Errorf("LogHasher(%s) is an unknown hasher", h)
}

// NewMapHasher returns a MapHasher.
func NewMapHasher(h trillian.HashStrategy) (hashers.MapHasher, error) {
	f := mapHashers[h]
	if f != nil {
		return f, nil
	}
	return nil, fmt.Errorf("MapHasher(%s) is an unknown hasher", h)
}
