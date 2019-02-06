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
    - [Proof](#trillian.Proof)
    - [QueueLeafRequest](#trillian.QueueLeafRequest)
    - [QueueLeafResponse](#trillian.QueueLeafResponse)
    - [QueueLeavesRequest](#trillian.QueueLeavesRequest)
    - [QueueLeavesResponse](#trillian.QueueLeavesResponse)
    - [QueuedLogLeaf](#trillian.QueuedLogLeaf)
  
  
  
    - [TrillianLog](#trillian.TrillianLog)
  

- [trillian_log_sequencer_api.proto](#trillian_log_sequencer_api.proto)
  
  
  
    - [TrillianLogSequencer](#trillian.TrillianLogSequencer)
  

- [trillian_map_api.proto](#trillian_map_api.proto)
    - [GetMapLeavesByRevisionRequest](#trillian.GetMapLeavesByRevisionRequest)
    - [GetMapLeavesRequest](#trillian.GetMapLeavesRequest)
    - [GetMapLeavesResponse](#trillian.GetMapLeavesResponse)
    - [GetSignedMapRootByRevisionRequest](#trillian.GetSignedMapRootByRevisionRequest)
    - [GetSignedMapRootRequest](#trillian.GetSignedMapRootRequest)
    - [GetSignedMapRootResponse](#trillian.GetSignedMapRootResponse)
    - [InitMapRequest](#trillian.InitMapRequest)
    - [InitMapResponse](#trillian.InitMapResponse)
    - [MapLeaf](#trillian.MapLeaf)
    - [MapLeafInclusion](#trillian.MapLeafInclusion)
    - [SetMapLeavesRequest](#trillian.SetMapLeavesRequest)
    - [SetMapLeavesResponse](#trillian.SetMapLeavesResponse)
  
  
  
    - [TrillianMap](#trillian.TrillianMap)
  

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
| proof | [Proof](#trillian.Proof) |  |  |
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
| leaf_hash | [bytes](#bytes) |  |  |
| tree_size | [int64](#int64) |  |  |
| order_by_sequence | [bool](#bool) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetInclusionProofByHashResponse"></a>

### GetInclusionProofByHashResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| proof | [Proof](#trillian.Proof) | repeated | Logs can potentially contain leaves with duplicate hashes so it&#39;s possible for this to return multiple proofs. |
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
| proof | [Proof](#trillian.Proof) |  |  |
| signed_log_root | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |






<a name="trillian.GetLatestSignedLogRootRequest"></a>

### GetLatestSignedLogRootRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetLatestSignedLogRootResponse"></a>

### GetLatestSignedLogRootResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| signed_log_root | [SignedLogRoot](#trillian.SignedLogRoot) |  |  |






<a name="trillian.GetLeavesByHashRequest"></a>

### GetLeavesByHashRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_id | [int64](#int64) |  |  |
| leaf_hash | [bytes](#bytes) | repeated |  |
| order_by_sequence | [bool](#bool) |  |  |
| charge_to | [ChargeTo](#trillian.ChargeTo) |  |  |






<a name="trillian.GetLeavesByHashResponse"></a>

### GetLeavesByHashResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| leaves | [LogLeaf](#trillian.LogLeaf) | repeated | TODO(gbelvin) reply with error codes. Reuse QueuedLogLeaf? |
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
| leaves | [LogLeaf](#trillian.LogLeaf) | repeated | TODO(gbelvin) reply with error codes. Reuse QueuedLogLeaf? |
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
A leaf of the log&#39;s Merkle tree, corresponds to a single log entry. Each leaf
has a unique `leaf_index` in the scope of this tree.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| merkle_leaf_hash | [bytes](#bytes) |  | Output only. The hash over `leaf_data`. |
| leaf_value | [bytes](#bytes) |  | Required. The arbitrary data associated with this log entry. Validity of this field is governed by the call site (personality). |
| extra_data | [bytes](#bytes) |  | The arbitrary metadata, e.g., a timestamp. |
| leaf_index | [int64](#int64) |  | Output only in `LOG` mode. Required in `PREORDERED_LOG` mode. The index of the leaf in the Merkle tree, i.e., the position of the corresponding entry in the log. For normal logs this value will be assigned by the LogSigner. |
| leaf_identity_hash | [bytes](#bytes) |  | The hash over the identity of this leaf. If empty, assumed to be the same as `merkle_leaf_hash`. It is a mechanism for the personality to provide a hint to Trillian that two leaves should be considered &#34;duplicates&#34; even though their `leaf_value`s differ.

E.g., in a CT personality multiple `add-chain` calls for an identical certificate would produce differing `leaf_data` bytes (due to the presence of SCT elements), with just this information Trillian would be unable to determine that. Within the context of the CT personality, these entries are dupes, so it sets `leaf_identity_hash` to `H(cert)`, which allows Trillian to detect the duplicates.

Continuing the CT example, for a CT mirror personality (which must allow dupes since the source log could contain them), the part of the personality which fetches and submits the entries might set `leaf_identity_hash` to `H(leaf_index||cert)`. TODO(pavelkalinnikov): Consider instead using `H(cert)` and allowing identity hash dupes in `PREORDERED_LOG` mode, for it can later be upgraded to `LOG` which will need to correctly detect duplicates with older entries when new ones get queued. |
| queue_timestamp | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  | Output only. The time at which this leaf was passed to `QueueLeaves`. This value will be determined and set by the LogServer. Equals zero if the entry was submitted without queuing. |
| integrate_timestamp | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  | Output only. The time at which this leaf was integrated into the tree. This value will be determined and set by the LogSigner. |






<a name="trillian.Proof"></a>

### Proof
A consistency or inclusion proof for a Merkle tree. Output only.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| leaf_index | [int64](#int64) |  |  |
| hashes | [bytes](#bytes) | repeated |  |






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
| queued_leaf | [QueuedLogLeaf](#trillian.QueuedLogLeaf) |  |  |






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
A result of submitting an entry to the log. Output only.
TODO(pavelkalinnikov): Consider renaming it to AddLogLeafResult or the like.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| leaf | [LogLeaf](#trillian.LogLeaf) |  | The leaf as it was stored by Trillian. Empty unless `status.code` is: - `google.rpc.OK`: the `leaf` data is the same as in the request. - `google.rpc.ALREADY_EXISTS` or &#39;google.rpc.FAILED_PRECONDITION`: the `leaf` is the conflicting one already in the log. |
| status | [google.rpc.Status](#google.rpc.Status) |  | The status of adding the leaf. - `google.rpc.OK`: successfully added. - `google.rpc.ALREADY_EXISTS`: the leaf is a duplicate of an already existing one. Either `leaf_identity_hash` is the same in the `LOG` mode, or `leaf_index` in the `PREORDERED_LOG`. - `google.rpc.FAILED_PRECONDITION`: A conflicting entry is already present in the log, e.g., same `leaf_index` but different `leaf_data`. |





 

 

 


<a name="trillian.TrillianLog"></a>

### TrillianLog
Provides access to a Verifiable Log data structure as defined in the
[Verifiable Data Structures](docs/papers/VerifiableDataStructures.pdf) paper.

The API supports adding new entries to be integrated into the log&#39;s tree. It
does not provide arbitrary tree modifications. Additionally, it has read
operations such as obtaining tree leaves, inclusion/consistency proofs etc.

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| QueueLeaf | [QueueLeafRequest](#trillian.QueueLeafRequest) | [QueueLeafResponse](#trillian.QueueLeafResponse) | Adds a single leaf to the queue. |
| AddSequencedLeaf | [AddSequencedLeafRequest](#trillian.AddSequencedLeafRequest) | [AddSequencedLeafResponse](#trillian.AddSequencedLeafResponse) | Adds a single leaf with an assigned sequence number. Warning: This RPC is under development, don&#39;t use it. |
| GetInclusionProof | [GetInclusionProofRequest](#trillian.GetInclusionProofRequest) | [GetInclusionProofResponse](#trillian.GetInclusionProofResponse) | Returns inclusion proof for a leaf with a given index in a given tree. |
| GetInclusionProofByHash | [GetInclusionProofByHashRequest](#trillian.GetInclusionProofByHashRequest) | [GetInclusionProofByHashResponse](#trillian.GetInclusionProofByHashResponse) | Returns inclusion proof for a leaf with a given Merkle hash in a given tree. |
| GetConsistencyProof | [GetConsistencyProofRequest](#trillian.GetConsistencyProofRequest) | [GetConsistencyProofResponse](#trillian.GetConsistencyProofResponse) | Returns consistency proof between two versions of a given tree. |
| GetLatestSignedLogRoot | [GetLatestSignedLogRootRequest](#trillian.GetLatestSignedLogRootRequest) | [GetLatestSignedLogRootResponse](#trillian.GetLatestSignedLogRootResponse) | Returns the latest signed log root for a given tree. Corresponds to the ReadOnlyLogTreeTX.LatestSignedLogRoot storage interface. |
| GetSequencedLeafCount | [GetSequencedLeafCountRequest](#trillian.GetSequencedLeafCountRequest) | [GetSequencedLeafCountResponse](#trillian.GetSequencedLeafCountResponse) | Returns the total number of leaves that have been integrated into the given tree. Corresponds to the ReadOnlyLogTreeTX.GetSequencedLeafCount storage interface. DO NOT USE - FOR DEBUGGING/TEST ONLY |
| GetEntryAndProof | [GetEntryAndProofRequest](#trillian.GetEntryAndProofRequest) | [GetEntryAndProofResponse](#trillian.GetEntryAndProofResponse) | Returns log entry and the corresponding inclusion proof for a given leaf index in a given tree. If the requested tree is unavailable but the leaf is in scope for the current tree, return a proof in that tree instead. |
| InitLog | [InitLogRequest](#trillian.InitLogRequest) | [InitLogResponse](#trillian.InitLogResponse) |  |
| QueueLeaves | [QueueLeavesRequest](#trillian.QueueLeavesRequest) | [QueueLeavesResponse](#trillian.QueueLeavesResponse) | Adds a batch of leaves to the queue. |
| AddSequencedLeaves | [AddSequencedLeavesRequest](#trillian.AddSequencedLeavesRequest) | [AddSequencedLeavesResponse](#trillian.AddSequencedLeavesResponse) | Stores leaves from the provided batch and associates them with the log positions according to the `LeafIndex` field. The indices must be contiguous.

Warning: This RPC is under development, don&#39;t use it. |
| GetLeavesByIndex | [GetLeavesByIndexRequest](#trillian.GetLeavesByIndexRequest) | [GetLeavesByIndexResponse](#trillian.GetLeavesByIndexResponse) | Returns a batch of leaves located in the provided positions. |
| GetLeavesByRange | [GetLeavesByRangeRequest](#trillian.GetLeavesByRangeRequest) | [GetLeavesByRangeResponse](#trillian.GetLeavesByRangeResponse) | Returns a batch of leaves in a sequential range. |
| GetLeavesByHash | [GetLeavesByHashRequest](#trillian.GetLeavesByHashRequest) | [GetLeavesByHashResponse](#trillian.GetLeavesByHashResponse) | Returns a batch of leaves by their `merkle_leaf_hash` values. |

 



<a name="trillian_log_sequencer_api.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## trillian_log_sequencer_api.proto


 

 

 


<a name="trillian.TrillianLogSequencer"></a>

### TrillianLogSequencer
The API supports sequencing in the Trillian Log Sequencer.

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|

 



<a name="trillian_map_api.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## trillian_map_api.proto



<a name="trillian.GetMapLeavesByRevisionRequest"></a>

### GetMapLeavesByRevisionRequest
This message replaces the current implementation of GetMapLeavesRequest
with the difference that revision must be &gt;=0.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| map_id | [int64](#int64) |  |  |
| index | [bytes](#bytes) | repeated |  |
| revision | [int64](#int64) |  | revision &gt;= 0. |






<a name="trillian.GetMapLeavesRequest"></a>

### GetMapLeavesRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| map_id | [int64](#int64) |  |  |
| index | [bytes](#bytes) | repeated |  |






<a name="trillian.GetMapLeavesResponse"></a>

### GetMapLeavesResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| map_leaf_inclusion | [MapLeafInclusion](#trillian.MapLeafInclusion) | repeated |  |
| map_root | [SignedMapRoot](#trillian.SignedMapRoot) |  |  |






<a name="trillian.GetSignedMapRootByRevisionRequest"></a>

### GetSignedMapRootByRevisionRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| map_id | [int64](#int64) |  |  |
| revision | [int64](#int64) |  |  |






<a name="trillian.GetSignedMapRootRequest"></a>

### GetSignedMapRootRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| map_id | [int64](#int64) |  |  |






<a name="trillian.GetSignedMapRootResponse"></a>

### GetSignedMapRootResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| map_root | [SignedMapRoot](#trillian.SignedMapRoot) |  |  |






<a name="trillian.InitMapRequest"></a>

### InitMapRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| map_id | [int64](#int64) |  |  |






<a name="trillian.InitMapResponse"></a>

### InitMapResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| created | [SignedMapRoot](#trillian.SignedMapRoot) |  |  |






<a name="trillian.MapLeaf"></a>

### MapLeaf
MapLeaf represents the data behind Map leaves.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [bytes](#bytes) |  | index is the location of this leaf. All indexes for a given Map must contain a constant number of bits. These are not numeric indices. Note that this is typically derived using a hash and thus the length of all indices in the map will match the number of bits in the hash function. Map entries do not have a well defined ordering and it&#39;s not possible to sequentially iterate over them. |
| leaf_hash | [bytes](#bytes) |  | leaf_hash is the tree hash of leaf_value. This does not need to be set on SetMapLeavesRequest; the server will fill it in. For an empty leaf (len(leaf_value)==0), there may be two possible values for this hash: - If the leaf has never been set, it counts as an empty subtree and a nil value is used. - If the leaf has been explicitly set to a zero-length entry, it no longer counts as empty and the value of hasher.HashLeaf(index, nil) will be used. |
| leaf_value | [bytes](#bytes) |  | leaf_value is the data the tree commits to. |
| extra_data | [bytes](#bytes) |  | extra_data holds related contextual data, but is not covered by any hash. |






<a name="trillian.MapLeafInclusion"></a>

### MapLeafInclusion



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| leaf | [MapLeaf](#trillian.MapLeaf) |  |  |
| inclusion | [bytes](#bytes) | repeated | inclusion holds the inclusion proof for this leaf in the map root. It holds one entry for each level of the tree; combining each of these in turn with the leaf&#39;s hash (according to the tree&#39;s hash strategy) reproduces the root hash. A nil entry for a particular level indicates that the node in question has an empty subtree beneath it (and so its associated hash value is hasher.HashEmpty(index, height) rather than hasher.HashChildren(l_hash, r_hash)). |






<a name="trillian.SetMapLeavesRequest"></a>

### SetMapLeavesRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| map_id | [int64](#int64) |  |  |
| leaves | [MapLeaf](#trillian.MapLeaf) | repeated | The leaves being set must have unique Index values within the request. |
| metadata | [bytes](#bytes) |  |  |
| revision | [int64](#int64) |  | The map revision to associate the leaves with. The request will fail if this revision already exists, does not match the current write revision, or is negative. If revision = 0 then the leaves will be written to the current write revision. |






<a name="trillian.SetMapLeavesResponse"></a>

### SetMapLeavesResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| map_root | [SignedMapRoot](#trillian.SignedMapRoot) |  |  |





 

 

 


<a name="trillian.TrillianMap"></a>

### TrillianMap
TrillianMap defines a service which provides access to a Verifiable Map as
defined in the Verifiable Data Structures paper.

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| GetLeaves | [GetMapLeavesRequest](#trillian.GetMapLeavesRequest) | [GetMapLeavesResponse](#trillian.GetMapLeavesResponse) | GetLeaves returns an inclusion proof for each index requested. For indexes that do not exist, the inclusion proof will use nil for the empty leaf value. |
| GetLeavesByRevision | [GetMapLeavesByRevisionRequest](#trillian.GetMapLeavesByRevisionRequest) | [GetMapLeavesResponse](#trillian.GetMapLeavesResponse) |  |
| SetLeaves | [SetMapLeavesRequest](#trillian.SetMapLeavesRequest) | [SetMapLeavesResponse](#trillian.SetMapLeavesResponse) | SetLeaves sets the values for the provided leaves, and returns the new map root if successful. Note that if a SetLeaves request fails for a server-side reason (i.e. not an invalid request), the API user is required to retry the request before performing a different SetLeaves request. |
| GetSignedMapRoot | [GetSignedMapRootRequest](#trillian.GetSignedMapRootRequest) | [GetSignedMapRootResponse](#trillian.GetSignedMapRootResponse) |  |
| GetSignedMapRootByRevision | [GetSignedMapRootByRevisionRequest](#trillian.GetSignedMapRootByRevisionRequest) | [GetSignedMapRootResponse](#trillian.GetSignedMapRootResponse) |  |
| InitMap | [InitMapRequest](#trillian.InitMapRequest) | [InitMapResponse](#trillian.InitMapResponse) |  |

 



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
| timestamp_nanos | [int64](#int64) |  | Deprecated: TimestampNanos moved to LogRoot. |
| root_hash | [bytes](#bytes) |  | Deprecated: RootHash moved to LogRoot. |
| tree_size | [int64](#int64) |  | Deprecated: TreeSize moved to LogRoot. |
| tree_revision | [int64](#int64) |  | Deprecated: TreeRevision moved to LogRoot. |
| key_hint | [bytes](#bytes) |  | key_hint is a hint to identify the public key for signature verification. key_hint is not authenticated and may be incorrect or missing, in which case all known public keys may be used to verify the signature. When directly communicating with a Trillian gRPC server, the key_hint will typically contain the LogID encoded as a big-endian 64-bit integer; however, in other contexts the key_hint is likely to have different contents (e.g. it could be a GUID, a URL &#43; TreeID, or it could be derived from the public key itself). |
| log_root | [bytes](#bytes) |  | log_root holds the TLS-serialization of the following structure (described in RFC5246 notation): Clients should validate log_root_signature with VerifySignedLogRoot before deserializing log_root. enum { v1(1), (65535)} Version; struct { uint64 tree_size; opaque root_hash&lt;0..128&gt;; uint64 timestamp_nanos; uint64 revision; opaque metadata&lt;0..65535&gt;; } LogRootV1; struct { Version version; select(version) { case v1: LogRootV1; } } LogRoot; |
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

| .proto Type | Notes | C++ Type | Java Type | Python Type |
| ----------- | ----- | -------- | --------- | ----------- |
| <a name="double" /> double |  | double | double | float |
| <a name="float" /> float |  | float | float | float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long |
| <a name="bool" /> bool |  | bool | boolean | boolean |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str |

