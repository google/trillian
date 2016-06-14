package server

import (
	"errors"

	"github.com/google/trillian"
	"golang.org/x/net/context"
)

var (
	// TODO: Delete when implementation done
	ErrNotImplemented = errors.New("Not yet implemented")
)

type trillianLogServer struct {
}

func (t *trillianLogServer) QueueLeaves(ctx context.Context, req *trillian.QueueLeavesRequest) (*trillian.QueueLeavesResponse, error) {
	return nil, ErrNotImplemented
}

func (t *trillianLogServer) GetInclusionProof(ctx context.Context, req *trillian.GetInclusionProofRequest) (*trillian.GetInclusionProofResponse, error) {
	return nil, ErrNotImplemented
}

func (t *trillianLogServer) GetConsistencyProof(ctx context.Context, req *trillian.GetConsistencyProofRequest) (*trillian.GetConsistencyProofResponse, error) {
	return nil, ErrNotImplemented
}

func (t *trillianLogServer) GetLatesSignedLogRoot(ctx context.Context, req *trillian.GetLatestSignedLogRootRequest) (*trillian.GetLatestSignedLogRootResponse, error) {
	return nil, ErrNotImplemented
}

func (t *trillianLogServer) GetSequencedLeafCount(ctx context.Context, req *trillian.GetSequencedLeafCountRequest) (*trillian.GetSequencedLeafCountResponse, error) {
	return nil, ErrNotImplemented
}

func (t *trillianLogServer) GetLeavesByIndex(ctx context.Context, req *trillian.GetLeavesByIndexRequest) (*trillian.GetLeavesByIndexResponse, error) {
	return nil, ErrNotImplemented
}

func (t *trillianLogServer) GetLeavesByHashRequest(ctx context.Context, req *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
	return nil, ErrNotImplemented
}
