# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [trillian_log_api.proto](#trillian_log_api.proto)
    - [AddSequencedLeafRequest](#trillian.AddSequencedLeafRequest)
    - [AddSequencedLeafResponse](#trillian.AddSequencedLeafResponse)
    - [AddSequencedLeavesRequest](#trillian.AddSequencedLeavesRequest)
    - [AddSequencedLeavesResponse](#trillian.AddSequencedLeavesResponse)
    - [ChargeTo](#trillian.ChargeTo)
    - [GetConsistencyProofRequest](#trillian.GetConsistencyProofRequest)
    - [GetConsistencyProofResponse](#trillian.GetConsistencyProofResponse)
    - [GetEntryAndProofRequest](#trillian.GetEntryAndProofRequest)
    - [GetEntryAndProofResponse](#trillian.GetEntryAndProofResponse)
    - [GetInclusionProofByHashRequest](#trillian.GetInclusionProofByHashRequest)
    - [GetInclusionProofByHashResponse](#trillian.GetInclusionProofByHashResponse)
    - [GetInclusionProofRequest](#trillian.GetInclusionProofRequest)
    - [GetInclusionProofResponse](#trillian.GetInclusionProofResponse)
    - [GetLatestSignedLogRootRequest](#trillian.GetLatestSignedLogRootRequest)
    - [GetLatestSignedLogRootResponse](#trillian.GetLatestSignedLogRootResponse)
    - [GetLeavesByHashRequest](#trillian.GetLeavesByHashRequest)
    - [GetLeavesByHashResponse](#trillian.GetLeavesByHashResponse)
    - [GetLeavesByIndexRequest](#trillian.GetLeavesByIndexRequest)
    - [GetLeavesByIndexResponse](#trillian.GetLeavesByIndexResponse)
    - [GetLeavesByRangeRequest](#trillian.GetLeavesByRangeRequest)
    - [GetLeavesByRangeResponse](#trillian.GetLeavesByRangeResponse)
    - [GetSequencedLeafCountRequest](#trillian.GetSequencedLeafCountRequest)
    - [GetSequencedLeafCountResponse](#trillian.GetSequencedLeafCountResponse)
    - [InitLogRequest](#trillian.InitLogRequest)
    - [InitLogResponse](#trillian.InitLogResponse)
    - [LogLeaf](#trillian.LogLeaf)
    - [QueueLeafRequest](#trillian.QueueLeafRequest)
    - [QueueLeafResponse](#trillian.QueueLeafResponse)
    - [QueueLeavesRequest](#trillian.QueueLeavesRequest)
    - [QueueLeavesResponse](#trillian.QueueLeavesResponse)
    - [QueuedLogLeaf](#trillian.QueuedLogLeaf)
  
    - [TrillianLog](#trillian.TrillianLog)
  
- [trillian_admin_api.proto](#trillian_admin_api.proto)
    - [CreateTreeRequest](#trillian.CreateTreeRequest)
    - [DeleteTreeRequest](#trillian.DeleteTreeRequest)
    - [GetTreeRequest](#trillian.GetTreeRequest)
    - [ListTreesRequest](#trillian.ListTreesRequest)
    - [ListTreesResponse](#trillian.ListTreesResponse)
    - [UndeleteTreeRequest](#trillian.UndeleteTreeRequest)
    - [UpdateTreeRequest](#trillian.UpdateTreeRequest)
  
    - [TrillianAdmin](#trillian.TrillianAdmin)
  
- [trillian.proto](#trillian.proto)
    - [Proof](#trillian.Proof)
    - [SignedEntryTimestamp](#trillian.SignedEntryTimestamp)
    - [SignedLogRoot](#trillian.SignedLogRoot)
    - [SignedMapRoot](#trillian.SignedMapRoot)
    - [Tree](#trillian.Tree)
  
    - [HashStrategy](#trillian.HashStrategy)
    - [LogRootFormat](#trillian.LogRootFormat)
    - [MapRootFormat](#trillian.MapRootFormat)
    - [TreeState](#trillian.TreeState)
    - [TreeType](#trillian.TreeType)
  
- [Scalar Value Types](#scalar-value-types)



<a name="trillian_log_api.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## trillian_log_api.proto



<a name="trillian.AddSequencedLeafRequest"></a>

### AddSequencedLeafRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| leaf | [LogLeaf](#trillian.LogLeaf) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.AddSequencedLeafResponse"></a>

### AddSequencedLeafResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| result | [QueuedLogLeaf](#trillian.QueuedLogLeaf) |  |  |






<a name="trillian.AddSequencedLeavesRequest"></a>

### AddSequencedLeavesRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| leaves | [LogLeaf](#trillian.LogLeaf) | repeated |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.AddSequencedLeavesResponse"></a>

### AddSequencedLeavesResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| results | [QueuedLogLeaf](#trillian.QueuedLogLeaf) | repeated | Same number and order as in the corresponding request. |






<a name="trillian.ChargeTo"></a>

### ChargeTo
ChargeTo describes the user(s) associated with the request whose quota should
be checked and charged.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| user | [string](#string) | repeated | user is a list of personality-defined strings. Trillian will treat them as /User/%{user}/... keys when checking and charging quota. If one or more of the specified users has insufficient quota, the request will be denied.

As an example, a Certificate Transparency frontend might set the following user strings when sending a QueueLeaves request to the Trillian log: - The requesting IP address. This would limit the number of requests per IP. - The &#34;intermediate-&lt;hash&gt;&#34; for each of the intermediate certificates in the submitted chain. This would have the effect of limiting the rate of submissions under a given intermediate/root. |






<a name="trillian.GetConsistencyProofRequest"></a>

### GetConsistencyProofRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| first_tree_size | [int64](#int64) |  |  |
| second_tree_size | [int64](#int64) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetConsistencyProofResponse"></a>

### GetConsistencyProofResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| proof | [Proof](#trillian.Proof) |  | The proof field may be empty if the requested tree_size was larger than that available at the server (e.g. because there is skew between server instances, and an earlier client request was processed by a more up-to-date instance). In this case, the signed_log_root field will indicate the tree size that the server is aware of, and the proof field will be empty. |
| signed_log_root | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |






<a name="trillian.GetEntryAndProofRequest"></a>

### GetEntryAndProofRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| leaf_index | [int64](#int64) |  |  |
| tree_size | [int64](#int64) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetEntryAndProofResponse"></a>

### GetEntryAndProofResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| proof | [Proof](#trillian.Proof) |  |  |
| leaf | [LogLeaf](#trillian.LogLeaf) |  |  |
| signed_log_root | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |






<a name="trillian.GetInclusionProofByHashRequest"></a>

### GetInclusionProofByHashRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| leaf_hash | [bytes](#bytes) |  | The leaf hash field provides the Merkle tree hash of the leaf entry to be retrieved. |
| tree_size | [int64](#int64) |  |  |
| order_by_sequence | [bool](#bool) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetInclusionProofByHashResponse"></a>

### GetInclusionProofByHashResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| proof | [Proof](#trillian.Proof) | repeated | Logs can potentially contain leaves with duplicate hashes so it&#39;s possible for this to return multiple proofs. If the leaf index for a particular instance of the requested Merkle leaf hash is beyond the requested tree size, the corresponding proof entry will be missing. |
| signed_log_root | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |






<a name="trillian.GetInclusionProofRequest"></a>

### GetInclusionProofRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| leaf_index | [int64](#int64) |  |  |
| tree_size | [int64](#int64) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetInclusionProofResponse"></a>

### GetInclusionProofResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| proof | [Proof](#trillian.Proof) |  | The proof field may be empty if the requested tree_size was larger than that available at the server (e.g. because there is skew between server instances, and an earlier client request was processed by a more up-to-date instance). In this case, the signed_log_root field will indicate the tree size that the server is aware of, and the proof field will be empty. |
| signed_log_root | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |






<a name="trillian.GetLatestSignedLogRootRequest"></a>

### GetLatestSignedLogRootRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |
| first_tree_size | [int64](#int64) |  | If first_tree_size is non-zero, the response will include a consistency proof between first_tree_size and the new tree size (if not smaller). |






<a name="trillian.GetLatestSignedLogRootResponse"></a>

### GetLatestSignedLogRootResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| signed_log_root | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |
| proof | [Proof](#trillian.Proof) |  | proof is filled in with a consistency proof if first_tree_size in GetLatestSignedLogRootRequest is non-zero (and within the tree size available at the server). |






<a name="trillian.GetLeavesByHashRequest"></a>

### GetLeavesByHashRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| leaf_hash | [bytes](#bytes) | repeated | The Merkle leaf hash of the leaf to be retrieved. |
| order_by_sequence | [bool](#bool) |  | If order_by_sequence is set then leaves will be returned in order of ascending leaf index. |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetLeavesByHashResponse"></a>

### GetLeavesByHashResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| leaves | [LogLeaf](#trillian.LogLeaf) | repeated |  |
| signed_log_root | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |






<a name="trillian.GetLeavesByIndexRequest"></a>

### GetLeavesByIndexRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| leaf_index | [int64](#int64) | repeated |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetLeavesByIndexResponse"></a>

### GetLeavesByIndexResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| leaves | [LogLeaf](#trillian.LogLeaf) | repeated | TODO(gbelvin): Response syntax does not allow for some requested leaves to be available, and some not (but using QueuedLogLeaf might) |
| signed_log_root | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |






<a name="trillian.GetLeavesByRangeRequest"></a>

### GetLeavesByRangeRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| start_index | [int64](#int64) |  |  |
| count | [int64](#int64) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetLeavesByRangeResponse"></a>

### GetLeavesByRangeResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| leaves | [LogLeaf](#trillian.LogLeaf) | repeated | Returned log leaves starting from the `start_index` of the request, in order. There may be fewer than `request.count` leaves returned, if the requested range extended beyond the size of the tree or if the server opted to return fewer leaves than requested. |
| signed_log_root | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |






<a name="trillian.GetSequencedLeafCountRequest"></a>

### GetSequencedLeafCountRequest
DO NOT USE - FOR DEBUGGING/TEST ONLY

(Use GetLatestSignedLogRoot then de-serialize the Log Root and use
use the tree size field within.)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetSequencedLeafCountResponse"></a>

### GetSequencedLeafCountResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| leaf_count | [int64](#int64) |  |  |






<a name="trillian.InitLogRequest"></a>

### InitLogRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.InitLogResponse"></a>

### InitLogResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| created | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |






<a name="trillian.LogLeaf"></a>

### LogLeaf
LogLeaf describes a leaf in the Log&#39;s Merkle tree, corresponding to a single log entry.
Each leaf has a unique leaf index in the scope of this tree.  Clients submitting new
leaf entries should only set the following fields:
  - leaf_value
  - extra_data (optionally)
  - leaf_identity_hash (optionally)
  - leaf_index (iff the log is a PREORDERED_LOG)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| merkle_leaf_hash | [bytes](#bytes) |  | merkle_leaf_hash holds the Merkle leaf hash over leaf_value. This is calculated by the Trillian server when leaves are added to the tree, using the defined hashing algorithm and strategy for the tree; as such, the client does not need to set it on leaf submissions. |
| leaf_value | [bytes](#bytes) |  | leaf_value holds the data that forms the value of the Merkle tree leaf. The client should set this field on all leaf submissions, and is responsible for ensuring its validity (the Trillian server treats it as an opaque blob). |
| extra_data | [bytes](#bytes) |  | extra_data holds additional data associated with the Merkle tree leaf. The client may set this data on leaf submissions, and the Trillian server will return it on subsequent read operations. However, the contents of this field are not covered by and do not affect the Merkle tree hash calculations. |
| leaf_index | [int64](#int64) |  | leaf_index indicates the index of this leaf in the Merkle tree. This field is returned on all read operations, but should only be set for leaf submissions in PREORDERED_LOG mode (for a normal log the leaf index is assigned by Trillian when the submitted leaf is integrated into the Merkle tree). |
| leaf_identity_hash | [bytes](#bytes) |  | leaf_identity_hash provides a hash value that indicates the client&#39;s concept of which leaf entries should be considered identical.

This mechanism allows the client personality to indicate that two leaves should be considered &#34;duplicates&#34; even though their `leaf_value`s differ.

If this is not set on leaf submissions, the Trillian server will take its value to be the same as merkle_leaf_hash (and thus only leaves with identical leaf_value contents will be considered identical).

For example, in Certificate Transparency each certificate submission is associated with a submission timestamp, but subsequent submissions of the same certificate should be considered identical. This is achieved by setting the leaf identity hash to a hash over (just) the certificate, whereas the Merkle leaf hash encompasses both the certificate and its submission time -- allowing duplicate certificates to be detected.

Continuing the CT example, for a CT mirror personality (which must allow dupes since the source log could contain them), the part of the personality which fetches and submits the entries might set `leaf_identity_hash` to `H(leaf_index||cert)`.

TODO(pavelkalinnikov): Consider instead using `H(cert)` and allowing identity hash dupes in `PREORDERED_LOG` mode, for it can later be upgraded to `LOG` which will need to correctly detect duplicates with older entries when new ones get queued. |
| queue_timestamp | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  | queue_timestamp holds the time at which this leaf was queued for inclusion in the Log, or zero if the entry was submitted without queuing. Clients should not set this field on submissions. |
| integrate_timestamp | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  | integrate_timestamp holds the time at which this leaf was integrated into the tree. Clients should not set this field on submissions. |






<a name="trillian.QueueLeafRequest"></a>

### QueueLeafRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| leaf | [LogLeaf](#trillian.LogLeaf) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.QueueLeafResponse"></a>

### QueueLeafResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| queued_leaf | [QueuedLogLeaf](#trillian.QueuedLogLeaf) |  | queued_leaf describes the leaf which is or will be incorporated into the Log. If the submitted leaf was already present in the Log (as indicated by its leaf identity hash), then the returned leaf will be the pre-existing leaf entry rather than the submitted leaf. |






<a name="trillian.QueueLeavesRequest"></a>

### QueueLeavesRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| leaves | [LogLeaf](#trillian.LogLeaf) | repeated |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.QueueLeavesResponse"></a>

### QueueLeavesResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| queued_leaves | [QueuedLogLeaf](#trillian.QueuedLogLeaf) | repeated | Same number and order as in the corresponding request. |






<a name="trillian.QueuedLogLeaf"></a>

### QueuedLogLeaf
QueuedLogLeaf provides the result of submitting an entry to the log.
TODO(pavelkalinnikov): Consider renaming it to AddLogLeafResult or the like.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| leaf | [LogLeaf](#trillian.LogLeaf) |  | The leaf as it was stored by Trillian. Empty unless `status.code` is: - `google.rpc.OK`: the `leaf` data is the same as in the request. - `google.rpc.ALREADY_EXISTS` or &#39;google.rpc.FAILED_PRECONDITION`: the `leaf` is the conflicting one already in the log. |
| status | [google.rpc.Status](#google.rpc.Status) |  | The status of adding the leaf. - `google.rpc.OK`: successfully added. - `google.rpc.ALREADY_EXISTS`: the leaf is a duplicate of an already existing one. Either `leaf_identity_hash` is the same in the `LOG` mode, or `leaf_index` in the `PREORDERED_LOG`. - `google.rpc.FAILED_PRECONDITION`: A conflicting entry is already present in the log, e.g., same `leaf_index` but different `leaf_data`. |





 

 

 


<a name="trillian.TrillianLog"></a>

### TrillianLog
The TrillianLog service provides access to an append-only Log data structure
as described in the [Verifiable Data
Structures](docs/papers/VerifiableDataStructures.pdf) paper.

The API supports adding new entries to the Merkle tree for a specific Log
instance (identified by its log_id) in two modes:
 - For a normal log, new leaf entries are queued up for subsequent
   inclusion in the log, and the leaves are assigned consecutive leaf_index
   values as part of that integration process.
 - For a &#39;pre-ordered log&#39;, new entries have an already-defined leaf
   ordering, and leaves are only integrated into the Merkle tree when a
   contiguous range of leaves is available.

The API also supports read operations to retrieve leaf contents, and to
provide cryptographic proofs of leaf inclusion and of the append-only nature
of the Log.

Each API request also includes a charge_to field, which allows API users
to provide quota identifiers that should be &#34;charged&#34; for each API request
(and potentially rejected with codes.ResourceExhausted).

Various operations on the API also allows for &#39;server skew&#39;, which can occur
when different API requests happen to be handled by different server instances
that may not all be up to date.  An API request that is relative to a specific
tree size may reach a server instance that is not yet aware of this tree size;
in this case the server will typically return an OK response that contains:
 - a signed log root that indicates the tree size that it is aware of
 - an empty response otherwise.

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| QueueLeaf | [QueueLeafRequest](#trillian.QueueLeafRequest) | [QueueLeafResponse](#trillian.QueueLeafResponse) | QueueLeaf adds a single leaf to the queue of pending leaves for a normal log. |
| AddSequencedLeaf | [AddSequencedLeafRequest](#trillian.AddSequencedLeafRequest) | [AddSequencedLeafResponse](#trillian.AddSequencedLeafResponse) | AddSequencedLeaf adds a single leaf with an assigned sequence number to a pre-ordered log. |
| GetInclusionProof | [GetInclusionProofRequest](#trillian.GetInclusionProofRequest) | [GetInclusionProofResponse](#trillian.GetInclusionProofResponse) | GetInclusionProof returns an inclusion proof for a leaf with a given index in a particular tree.

If the requested tree_size is larger than the server is aware of, the response will include the latest known log root and an empty proof. |
| GetInclusionProofByHash | [GetInclusionProofByHashRequest](#trillian.GetInclusionProofByHashRequest) | [GetInclusionProofByHashResponse](#trillian.GetInclusionProofByHashResponse) | GetInclusionProofByHash returns an inclusion proof for any leaves that have the given Merkle hash in a particular tree.

If any of the leaves that match the given Merkle has have a leaf index that is beyond the requested tree size, the corresponding proof entry will be empty. |
| GetConsistencyProof | [GetConsistencyProofRequest](#trillian.GetConsistencyProofRequest) | [GetConsistencyProofResponse](#trillian.GetConsistencyProofResponse) | GetConsistencyProof returns a consistency proof between different sizes of a particular tree.

If the requested tree size is larger than the server is aware of, the response will include the latest known log root and an empty proof. |
| GetLatestSignedLogRoot | [GetLatestSignedLogRootRequest](#trillian.GetLatestSignedLogRootRequest) | [GetLatestSignedLogRootResponse](#trillian.GetLatestSignedLogRootResponse) | GetLatestSignedLogRoot returns the latest signed log root for a given tree, and optionally also includes a consistency proof from an earlier tree size to the new size of the tree.

If the earlier tree size is larger than the server is aware of, an InvalidArgument error is returned. |
| GetSequencedLeafCount | [GetSequencedLeafCountRequest](#trillian.GetSequencedLeafCountRequest) | [GetSequencedLeafCountResponse](#trillian.GetSequencedLeafCountResponse) | GetSequencedLeafCount returns the total number of leaves that have been integrated into the given tree.

DO NOT USE - FOR DEBUGGING/TEST ONLY

(Use GetLatestSignedLogRoot then de-serialize the Log Root and use use the tree size field within.) |
| GetEntryAndProof | [GetEntryAndProofRequest](#trillian.GetEntryAndProofRequest) | [GetEntryAndProofResponse](#trillian.GetEntryAndProofResponse) | GetEntryAndProof returns a log leaf and the corresponding inclusion proof to a specified tree size, for a given leaf index in a particular tree.

If the requested tree size is unavailable but the leaf is in scope for the current tree, the returned proof will be for the current tree size rather than the requested tree size. |
| InitLog | [InitLogRequest](#trillian.InitLogRequest) | [InitLogResponse](#trillian.InitLogResponse) | InitLog initializes a particular tree, creating the initial signed log root (which will be of size 0). |
| QueueLeaves | [QueueLeavesRequest](#trillian.QueueLeavesRequest) | [QueueLeavesResponse](#trillian.QueueLeavesResponse) | QueueLeaf adds a batch of leaves to the queue of pending leaves for a normal log. |
| AddSequencedLeaves | [AddSequencedLeavesRequest](#trillian.AddSequencedLeavesRequest) | [AddSequencedLeavesResponse](#trillian.AddSequencedLeavesResponse) | AddSequencedLeaves adds a batch of leaves with assigned sequence numbers to a pre-ordered log. The indices of the provided leaves must be contiguous. |
| GetLeavesByIndex | [GetLeavesByIndexRequest](#trillian.GetLeavesByIndexRequest) | [GetLeavesByIndexResponse](#trillian.GetLeavesByIndexResponse) | GetLeavesByIndex returns a batch of leaves whose leaf indices are provided in the request. |
| GetLeavesByRange | [GetLeavesByRangeRequest](#trillian.GetLeavesByRangeRequest) | [GetLeavesByRangeResponse](#trillian.GetLeavesByRangeResponse) | GetLeavesByRange returns a batch of leaves whose leaf indices are in a sequential range. |
| GetLeavesByHash | [GetLeavesByHashRequest](#trillian.GetLeavesByHashRequest) | [GetLeavesByHashResponse](#trillian.GetLeavesByHashResponse) | GetLeavesByHash returns a batch of leaves which are identified by their Merkle leaf hash values. |

 



<a name="trillian_admin_api.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## trillian_admin_api.proto



<a name="trillian.CreateTreeRequest"></a>

### CreateTreeRequest
CreateTree request.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tree | [Tree](#trillian.Tree) |  | Tree to be created. See Tree and CreateTree for more details. |
| key_spec | [keyspb.Specification](#keyspb.Specification) |  | Describes how the tree&#39;s private key should be generated. Only needs to be set if tree.private_key is not set. |






<a name="trillian.DeleteTreeRequest"></a>

### DeleteTreeRequest
DeleteTree request.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tree_id | [int64](#int64) |  | ID of the tree to delete. |






<a name="trillian.GetTreeRequest"></a>

### GetTreeRequest
GetTree request.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tree_id | [int64](#int64) |  | ID of the tree to retrieve. |






<a name="trillian.ListTreesRequest"></a>

### ListTreesRequest
ListTrees request.
No filters or pagination options are provided.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| show_deleted | [bool](#bool) |  | If true, deleted trees are included in the response. |






<a name="trillian.ListTreesResponse"></a>

### ListTreesResponse
ListTrees response.
No pagination is provided, all trees the requester has access to are
returned.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tree | [Tree](#trillian.Tree) | repeated | Trees matching the list request filters. |






<a name="trillian.UndeleteTreeRequest"></a>

### UndeleteTreeRequest
UndeleteTree request.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tree_id | [int64](#int64) |  | ID of the tree to undelete. |






<a name="trillian.UpdateTreeRequest"></a>

### UpdateTreeRequest
UpdateTree request.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tree | [Tree](#trillian.Tree) |  | Tree to be updated. |
| update_mask | [google.protobuf.FieldMask](#google.protobuf.FieldMask) |  | Fields modified by the update request. For example: &#34;tree_state&#34;, &#34;display_name&#34;, &#34;description&#34;. |





 

 

 


<a name="trillian.TrillianAdmin"></a>

### TrillianAdmin
Trillian Administrative interface.
Allows creation and management of Trillian trees (both log and map trees).

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| ListTrees | [ListTreesRequest](#trillian.ListTreesRequest) | [ListTreesResponse](#trillian.ListTreesResponse) | Lists all trees the requester has access to. |
| GetTree | [GetTreeRequest](#trillian.GetTreeRequest) | [Tree](#trillian.Tree) | Retrieves a tree by ID. |
| CreateTree | [CreateTreeRequest](#trillian.CreateTreeRequest) | [Tree](#trillian.Tree) | Creates a new tree. System-generated fields are not required and will be ignored if present, e.g.: tree_id, create_time and update_time. Returns the created tree, with all system-generated fields assigned. |
| UpdateTree | [UpdateTreeRequest](#trillian.UpdateTreeRequest) | [Tree](#trillian.Tree) | Updates a tree. See Tree for details. Readonly fields cannot be updated. |
| DeleteTree | [DeleteTreeRequest](#trillian.DeleteTreeRequest) | [Tree](#trillian.Tree) | Soft-deletes a tree. A soft-deleted tree may be undeleted for a certain period, after which it&#39;ll be permanently deleted. |
| UndeleteTree | [UndeleteTreeRequest](#trillian.UndeleteTreeRequest) | [Tree](#trillian.Tree) | Undeletes a soft-deleted a tree. A soft-deleted tree may be undeleted for a certain period, after which it&#39;ll be permanently deleted. |

 



<a name="trillian.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## trillian.proto



<a name="trillian.Proof"></a>

### Proof
Proof holds a consistency or inclusion proof for a Merkle tree, as returned
by the API.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| leaf_index | [int64](#int64) |  | leaf_index indicates the requested leaf index when this message is used for a leaf inclusion proof. This field is set to zero when this message is used for a consistency proof. |
| hashes | [bytes](#bytes) | repeated |  |






<a name="trillian.SignedEntryTimestamp"></a>

### SignedEntryTimestamp



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| timestamp_nanos | [int64](#int64) |  |  |
| log_id | [int64](#int64) |  |  |
| signature | [sigpb.DigitallySigned](#sigpb.DigitallySigned) |  |  |






<a name="trillian.SignedLogRoot"></a>

### SignedLogRoot
SignedLogRoot represents a commitment by a Log to a particular tree.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key_hint | [bytes](#bytes) |  | key_hint is a hint to identify the public key for signature verification. key_hint is not authenticated and may be incorrect or missing, in which case all known public keys may be used to verify the signature. When directly communicating with a Trillian gRPC server, the key_hint will typically contain the LogID encoded as a big-endian 64-bit integer; however, in other contexts the key_hint is likely to have different contents (e.g. it could be a GUID, a URL &#43; TreeID, or it could be derived from the public key itself). |
| log_root | [bytes](#bytes) |  | log_root holds the TLS-serialization of the following structure (described in RFC5246 notation): Clients should validate log_root_signature with VerifySignedLogRoot before deserializing log_root. enum { v1(1), (65535)} Version; struct { uint64 tree_size; opaque root_hash&lt;0..128&gt;; uint64 timestamp_nanos; uint64 revision; opaque metadata&lt;0..65535&gt;; } LogRootV1; struct { Version version; select(version) { case v1: LogRootV1; } } LogRoot;

A serialized v1 log root will therefore be laid out as:

&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;-....--&#43; | ver=1 | tree_size |len| root_hash | &#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;-....--&#43;

&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43; | timestamp_nanos | revision | &#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;---&#43;

&#43;---&#43;---&#43;---&#43;---&#43;---&#43;-....---&#43; | len | metadata | &#43;---&#43;---&#43;---&#43;---&#43;---&#43;-....---&#43;

(with all integers encoded big-endian). |
| log_root_signature | [bytes](#bytes) |  | log_root_signature is the raw signature over log_root. |






<a name="trillian.SignedMapRoot"></a>

### SignedMapRoot
SignedMapRoot represents a commitment by a Map to a particular tree.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| map_root | [bytes](#bytes) |  | map_root holds the TLS-serialization of the following structure (described in RFC5246 notation): Clients should validate signature with VerifySignedMapRoot before deserializing map_root. enum { v1(1), (65535)} Version; struct { opaque root_hash&lt;0..128&gt;; uint64 timestamp_nanos; uint64 revision; opaque metadata&lt;0..65535&gt;; } MapRootV1; struct { Version version; select(version) { case v1: MapRootV1; } } MapRoot; |
| signature | [bytes](#bytes) |  | Signature is the raw signature over MapRoot. |






<a name="trillian.Tree"></a>

### Tree
Represents a tree, which may be either a verifiable log or map.
Readonly attributes are assigned at tree creation, after which they may not
be modified.

Note: Many APIs within the rest of the code require these objects to
be provided. For safety they should be obtained via Admin API calls and
not created dynamically.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tree_id | [int64](#int64) |  | ID of the tree. Readonly. |
| tree_state | [TreeState](#trillian.TreeState) |  | State of the tree. Trees are ACTIVE after creation. At any point the tree may transition between ACTIVE, DRAINING and FROZEN states. |
| tree_type | [TreeType](#trillian.TreeType) |  | Type of the tree. Readonly after Tree creation. Exception: Can be switched from PREORDERED_LOG to LOG if the Tree is and remains in the FROZEN state. |
| hash_strategy | [HashStrategy](#trillian.HashStrategy) |  | Hash strategy to be used by the tree. Readonly. |
| hash_algorithm | [sigpb.DigitallySigned.HashAlgorithm](#sigpb.DigitallySigned.HashAlgorithm) |  | Hash algorithm to be used by the tree. Readonly. |
| signature_algorithm | [sigpb.DigitallySigned.SignatureAlgorithm](#sigpb.DigitallySigned.SignatureAlgorithm) |  | Signature algorithm to be used by the tree. Readonly. |
| display_name | [string](#string) |  | Display name of the tree. Optional. |
| description | [string](#string) |  | Description of the tree, Optional. |
| private_key | [google.protobuf.Any](#google.protobuf.Any) |  | Identifies the private key used for signing tree heads and entry timestamps. This can be any type of message to accommodate different key management systems, e.g. PEM files, HSMs, etc. Private keys are write-only: they&#39;re never returned by RPCs. The private_key message can be changed after a tree is created, but the underlying key must remain the same - this is to enable migrating a key from one provider to another. |
| storage_settings | [google.protobuf.Any](#google.protobuf.Any) |  | Storage-specific settings. Varies according to the storage implementation backing Trillian. |
| public_key | [keyspb.PublicKey](#keyspb.PublicKey) |  | The public key used for verifying tree heads and entry timestamps. Readonly. |
| max_root_duration | [google.protobuf.Duration](#google.protobuf.Duration) |  | Interval after which a new signed root is produced even if there have been no submission. If zero, this behavior is disabled. |
| create_time | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  | Time of tree creation. Readonly. |
| update_time | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  | Time of last tree update. Readonly (automatically assigned on updates). |
| deleted | [bool](#bool) |  | If true, the tree has been deleted. Deleted trees may be undeleted during a certain time window, after which they&#39;re permanently deleted (and unrecoverable). Readonly. |
| delete_time | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  | Time of tree deletion, if any. Readonly. |





 


<a name="trillian.HashStrategy"></a>

### HashStrategy
Defines the way empty / node / leaf hashes are constructed incorporating
preimage protection, which can be application specific.

| Name | Number | Description |
| ---- | ------ | ----------- |
| UNKNOWN_HASH_STRATEGY | 0 | Hash strategy cannot be determined. Included to enable detection of mismatched proto versions being used. Represents an invalid value. |
| RFC6962_SHA256 | 1 | Certificate Transparency strategy: leaf hash prefix = 0x00, node prefix = 0x01, empty hash is digest([]byte{}), as defined in the specification. |
| TEST_MAP_HASHER | 2 | Sparse Merkle Tree strategy: leaf hash prefix = 0x00, node prefix = 0x01, empty branch is recursively computed from empty leaf nodes. NOT secure in a multi tree environment. For testing only. |
| OBJECT_RFC6962_SHA256 | 3 | Append-only log strategy where leaf nodes are defined as the ObjectHash. All other properties are equal to RFC6962_SHA256. |
| CONIKS_SHA512_256 | 4 | The CONIKS sparse tree hasher with SHA512_256 as the hash algorithm. |
| CONIKS_SHA256 | 5 | The CONIKS sparse tree hasher with SHA256 as the hash algorithm. |



<a name="trillian.LogRootFormat"></a>

### LogRootFormat
LogRootFormat specifies the fields that are covered by the
SignedLogRoot signature, as well as their ordering and formats.

| Name | Number | Description |
| ---- | ------ | ----------- |
| LOG_ROOT_FORMAT_UNKNOWN | 0 |  |
| LOG_ROOT_FORMAT_V1 | 1 |  |



<a name="trillian.MapRootFormat"></a>

### MapRootFormat
MapRootFormat specifies the fields that are covered by the
SignedMapRoot signature, as well as their ordering and formats.

| Name | Number | Description |
| ---- | ------ | ----------- |
| MAP_ROOT_FORMAT_UNKNOWN | 0 |  |
| MAP_ROOT_FORMAT_V1 | 1 |  |



<a name="trillian.TreeState"></a>

### TreeState
State of the tree.

| Name | Number | Description |
| ---- | ------ | ----------- |
| UNKNOWN_TREE_STATE | 0 | Tree state cannot be determined. Included to enable detection of mismatched proto versions being used. Represents an invalid value. |
| ACTIVE | 1 | Active trees are able to respond to both read and write requests. |
| FROZEN | 2 | Frozen trees are only able to respond to read requests, writing to a frozen tree is forbidden. Trees should not be frozen when there are entries in the queue that have not yet been integrated. See the DRAINING state for this case. |
| DEPRECATED_SOFT_DELETED | 3 | Deprecated: now tracked in Tree.deleted. |
| DEPRECATED_HARD_DELETED | 4 | Deprecated: now tracked in Tree.deleted. |
| DRAINING | 5 | A tree that is draining will continue to integrate queued entries. No new entries should be accepted. |



<a name="trillian.TreeType"></a>

### TreeType
Type of the tree.

| Name | Number | Description |
| ---- | ------ | ----------- |
| UNKNOWN_TREE_TYPE | 0 | Tree type cannot be determined. Included to enable detection of mismatched proto versions being used. Represents an invalid value. |
| LOG | 1 | Tree represents a verifiable log. |
| MAP | 2 | Tree represents a verifiable map. |
| PREORDERED_LOG | 3 | Tree represents a verifiable pre-ordered log, i.e., a log whose entries are placed according to sequence numbers assigned outside of Trillian. |


 

 

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

