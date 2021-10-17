# Trillian Personalities

 - [Overview](#overview)
 - [Conceptual Responsibilities](#conceptual-responsibilities)
    - [Admission Control](#admission-control)
    - [Canonicalization](#canonicalization)
    - [Auditability](#auditability)
 - [Practical Responsibilities](#practical-responsibilities)
    - [External API](#external-api)
    - [Storage](#storage)
    - [Traffic Control](#traffic-control)
    - [Monitoring](#monitoring)
 - [Examples](#examples)
    - [Certificate Transparency](#certificate-transparency)
    - [Gossip Hub](#gossip-hub)

## Overview

The core Trillian service provides a Merkle tree store (and primitives to
interact with it), but a full transparency application requires additional
functionality on top of that base, which needs to be provided by a separate
*personality*.

The following sections describe the conceptual responsibilities of a Trillian
personality, followed by the practical responsibilities that are likely to be
needed for real-world use. Overall design considerations for transparent
Log applications are also
[discussed in a separate document](TransparentLogging.md).


## Conceptual Responsibilities

### Admission Control

The primary purpose of a personality is to implement **admission criteria** for
the store, so that only particular types of data are added to the store. For
example, a Certificate Transparency Log only accepts data items that are valid
certificates; a "CT Log" personality would police this, so that the Trillian
service can process all incoming data blindly.

It is common for these admission criteria to be broken into two distinct but
related areas:

 - **Structure**: Submissions to the personality have to be in a well known
   format, thus limiting the transparent data to a particular field of
   interest.  (For example, for CT the structure must be a DER-encoded X.509
   certificate.)
 - **Authentication**: Submissions to the personality may have to be
   signed by a known authority, in which case the personality must be
   configured with a set of acceptable authorities.  (For CT, each
   submission must chain to one of a set of accepted web PKI root
   certificates.)

### Canonicalization

A personality may also perform **canonicalization** on incoming data, to
convert equivalent formulations of the same underlying data to a single
canonical format, avoiding needless duplication. For example, keys in JSON
dictionaries could be sorted, or Unicode string data could be normalised.

One particularly important form of canonicalization is **de-duplication** of
timestamped data: if the Merkle leaf contents includes a timestamp indicating
when the leaf was added, subsequent submissions of the same data with a new
timestamp should not be treated as a new leaf.

This use case is supported by the Trillian API, which includes the concept of
an *identity hash* associated with a data item: items with the same identity
hash are assumed equal, even if the full Merkle leaf structure differs
(e.g. because it has a different timestamp attached).

This means that a personality that wants to duplicate entries can
do so by setting the `LeafIdentityHash` value on new Merkle tree leaves
appropriately. The personality code also needs to cope with duplicates: the
Merkle tree leaf in a successful `QueueLeaves` response may not be the same
as the tree leaf in the corresponding `QueueLeaves` request (it will have the
same `LeafIdentityHash` but may differ in fields that are not covered by this
hash, e.g. it may have an earlier timestamp).


### Auditability

If the personality and the Trillian core services are maintained by different
operators, then there may be an implicit trust boundary between the two at the
Trillian gRPC interface.

If this is the case, then the personality may need to be responsible for
storing data that allows auditing of that trust boundary.

For example, if an external monitor detects that a signed tree head is not
consistent with an earlier signed tree head, is there enough information
available to determine whether this is a problem with the personality or with
the Trillian service?

If the personality maintains a store of the signed log roots provided by
Trillian, it can then use this to audit failure cases and assign blame
appropriately – effectively acting as a monitor for Trillian.


## Practical Responsibilities

### External API

In many environments the personality provides an externally-visible API.
The design of such an API depends on the application, but key points to
consider include:

 - **syntax** – the mechanism used to access the API, where possibilities
   include:
   - HTTPS + JSON: A web-based interface has the advantage of being
     accessible using standard tools, ranging from
     [standard](https://golang.org/pkg/net/http/)
     [libraries](https://docs.python.org/3/library/http.client.html) to
     command-line tools like `curl` and `wget`.  (The Certificate Transparency
     API described in [RFC 6962](https://tools.ietf.org/html/rfc6962) takes
     this approach.)
   - gRPC: An RPC-based interface allows for additional tooling (such as
     mechanical checks of schema adherence) and may allow features that are not
     easily achieved in a RESTful web API (such as
     [continuous streaming](https://grpc.io/docs/guides/concepts.html#server-streaming-rpc)
     of data).

 - **semantics** – the set of primitives and data structures exposed by
   the API, for example:
   - What submission formats are accepted?
   - What cryptographic promises/guarantees does the personality generate?
   - Which Merkle tree primitives are exposed to external users?


### Storage

A Trillian personality may need an additional storage mechanism over and above
the Merkle tree storage provided by Trillian.  For example, if Trillian is run
by a separate operator, then
[audit verification data may need to be stored](#auditability).

However, note that the Trillian APIs allow a personality to store additional
data that is associated with a leaf in the underlying Merkle tree, in the
`ExtraData` field.  This extra data is immutable but is not covered by the
Merkle tree hashing algorithms, and so cannot be used for
verifiable/cryptographic purposes (as it could be modified without affecting
tree hash values).


### Traffic Control

The core Trillian service can be configured to provide traffic control limits
using its [quota system](https://godoc.org/github.com/google/trillian/quota);
out of the box, this can be governed by:
 - global rate limits (for both read and write operations)
 - per-Merkle tree rate limits (for read and write).

A particular application may also want to provide other kinds of limits.  This
is supported by the per-*user* quota limits, where the *user* is entirely
specified by the Trillian personality, using the `ChargeTo` field in each
Trillian API request.

A typical use case for this is to add a `ChargeTo.User` string that identifies
an external requestor, so traffic from a single source can be limited without
affecting other users.

However, the user quota system can also be used for more flexible limits – for
example, by applying limits to particular authentication keys.


### Monitoring

A personality that is in real-world use should probably be monitored, for
performance, alerting and debugging.  Trillian uses a
[metrics interface](../monitoring/metrics.go) to support this, which can also be
re-used in an associated personality.

## Examples

This section describes two Trillian personalities, together with their choices
for the various design decisions described above.

### Certificate Transparency

Certificate Transparency (CT) is the original transparency application, and has
been successful in improving the security of the web PKI ecosystem.  It is also
the most mature and widely deployed Trillian use case, built on the
[CTFE personality](https://github.com/google/certificate-transparency-go/blob/master/trillian/ctfe).

The [conceptual responsibilities](#conceptual-responsibilities) of the CTFE
personality are given by:

 - Admission Control: Submissions have to take the form of a chain of valid
   X.509 certificates, each certificate signed by the next certificate in the
   chain, until a acceptable root certificate is reached.  The set of accepted
   roots is configured in the personality at startup.
 - Canonicalization: Submissions have a `LeafIdentityHash` which covers the
   leaf certificate, but also have a timestamp generated by the CTFE. A
   re-submission of a previously-seen certificate will return the original
   leaf with its earlier timestamp.
 - Auditability: The CTFE personality assumes that the Trillian service is run
   by the same operator, so does not store personality-to-Trillian audit data.

In terms of [practical responsibilities](#practical-responsibilities):

 - The external API to the CTFE personality is the HTTP(S)+JSON interface
   specified by [RFC 6962](https://tools.ietf.org/html/rfc6962), in both syntax
   and semantics.  The CTFE personality is configured with private key material
   for each CT Log instance, and uses this to generate those API data
   structures that are signed – the Signed Tree Head (STH) and Signed
   Certificate Timestamp (SCT).
 - The CTFE personality does not require storage independent of the core Merkle
   tree.  The chain of certificates used to validate the original submission of
   a leaf certificate are stored as the `ExtraData` associated with a Merkle
   tree leaf.
 - The CTFE personality can be configured to charge API requests to per-user
   quota for:
     - The IP address of the original requestor.
     - The set of intermediate and root certificates used to validate a
       submission.
 - Metrics are exposed using Trillian's metrics library, and sample Prometheus
   consoles and scripts are provided.


### Gossip Hub

Users of individual transparent Logs are theoretically susceptible to a
*split-view* attack, where a Log presents a different view of its Merkle tree
to different users – each user's view of the tree is internally consistent, but
differs between users.

A *gossip protocol* is a key mechanism to prevent this: different observers of
a transparent Log share their views of the Log, allowing cross-comparison and
eventual detection of split views.

The
[Gossip Hub](https://github.com/google/trillian-examples/blob/master/gossip) is
an **experimental** implementation of a repository for information about a Log's
Merkle tree, in the form of (another) transparent Log.  This allows
cross-comparison of the source Log's Merkle tree: different observers can
submit their view of the source Log, and retrieve other observers' views of the
same source Log.

(Aside: in practice, split-view attacks are less of a concern for Certificate
Transparency and the web PKI because browser policies typically require
individual certificates to be logged in multiple independent CT Logs, limiting
the scope of any individual Log's split-view attack.)

The [conceptual responsibilities](#conceptual-responsibilities) of the Gossip
Hub personality are given by:

 - Admission Control: Submissions take the form of signed blobs of data, and
   the data may be required to take the form of a valid RFC 6962 signed tree
   head structure.  The set of accepted public keys from the source Logs, and
   the expected submission structure, is configured in the personality at
   start-of-day.
 - Canonicalization: Submissions have a `LeafIdentityHash` which covers the
   submitted data and signature, but also have a timestamp generated by the
   Hub. A re-submission of a previously-seen entry will return the original
   leaf with its earlier timestamp.
 - Auditability: The Hub personality assumes that the Trillian service is run by
   the same operator, so does not store personality-to-Trillian audit data.

In terms of [practical responsibilities](#practical-responsibilities):

 - The external API to the Gossip Hub personality is an
   [HTTP(S)+JSON interface](https://github.com/google/trillian-examples/blob/master/gossip/api/types.go)
   analogous to that specified by
   [RFC 6962](https://tools.ietf.org/html/rfc6962), in both syntax and
   semantics.  The Hub personality is configured with private key material for
   each Hub instance, and uses this to generate signed API data structures – the
   Signed Tree Head (STH) and Signed Gossip Timestamp (SGT).
 - The Hub personality does not require storage independent of the core Merkle
   tree, and the `ExtraData` field is not (currently) used.
 - The Hub personality can be configured to charge API requests to per-user
   quota for:
     - The IP address of the original requestor.
     - The public key used to sign a submission.
 - Metrics are exposed using Trillian's metrics library.
