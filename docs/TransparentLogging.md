# Transparent Logging: A Guide

 - [Introduction](#introduction)
   - [Running Examples](#running-examples)
 - [Ecosystem](#ecosystem)
 - [Log Contents](#log-contents)
   - [Leaf Hashing](#leaf-hashing)
   - [Leaf Size / Accessibility](#leaf-size--accessibility)
 - [Admission Control](#admission-control)
 - [Inclusion Proofs vs. Promises](#inclusion-proofs-vs-promises)

## Introduction

The [Trillian project](https://github.com/google/trillian) generalizes the
ideas behind [Certificate Transparency](https://certificate-transparency.org),
to allow transparent, append-only logging of arbitrary data.

This document works through some of the high-level design decisions involved in
creating a transparent Log; it's a good idea to have a clear understanding of
these decisions before writing any code.

We assume the reader has a rough idea of the concepts involved in transparent
Logs: [Merkle trees](https://en.wikipedia.org/wiki/Merkle_tree),
[inclusion/consistency proofs](http://www.certificate-transparency.org/log-proofs-work),
[signed tree heads](https://tools.ietf.org/html/rfc6962#section-3.5) etc.


### Running Examples

Throughout this document, we will use three example scenarios for the use of a
Log, to illustrate the considerations involved:

 - **Certificate Transparency**:  The original transparent Log application,
   which allows logging of X.509 Certificates from Web PKI infrastructure.
 - **Binary Transparency**: A (so far) theoretical example where binary
   installable packages for a particular environment (e.g. OS+architecture) are
   logged.
 - **Pastebin**: Another theoretical example, of an open site that allows
   logging of arbitrary text blobs.

Two of these examples will turn out to be a good match for transparent Logging;
the other, not so much.

## Ecosystem

The first fundamental question for designing a transparent Log is: **Why are you
logging?**

Understanding what the Log is intended to achieve allows you to check whether
the transparent, append-only characteristics of the Log actually achieve those
goals.  It also helps you to understand how much of a wider ecosystem –
auditors, monitors, indexes – also needs to be built up around the Log.

 - The
   [rationale for Certificate Transparency](https://tools.ietf.org/html/rfc6962)
   is to log "certificates as they are issued or observed, in a manner that
   allows anyone to audit certificate authority activity and notice the issuance
   of suspect certificates".

## Log Contents

The second fundamental question for designing a transparent Log is: **What are you
logging?**

Ideally, you will have a single sentence that answers this question, and answers
it sufficiently clearly that there is little ambiguity left in the
implementation – the definition of the **leaf contents** for the Log should be
mostly covered by this sentence.

For example:

 - "*Certificate Transparency logs publicly trusted X.509 certificates*": The
   [leaf contents](https://github.com/google/certificate-transparency/blob/master/docs/images/RFC6962Structures.png)
   are essentially a certificate (plus a timestamp).
 - "*Binary Transparency logs binary packages that can be installed on XYZ
   architecture devices running ABC operating system*": The main part of the leaf
   contents would be the binary blob itself (or a hash of the binary blob to
   reduce leaf size).
 - "*Pastebin logs arbitrary text*": The leaf contents are just text.

Reality soon intrudes and dilutes the clarity of these statements, but they
remain useful as a guiding principle for what is and is not allowed as a
variation on the log contents.

 - For Certificate Transparency, it's helpful to allow transparency
   proofs-of-logging to be included in the certificates themselves.  However,
   this gives a chicken-and-egg problem: a certificate can only get a
   proof-of-logging after it's logged, and adding the proof changes the
   certificate.  The solution to this quandary is
   [precertificates](https://github.com/google/certificate-transparency/blob/master/docs/SCTValidation.md#precertificates):
   a commitment to certificate issuance that can be logged in place of the real
   certificate.
 - For Binary Transparency, it might be the case that packages can arrive as
   either complete images, or deltas on a previous image. For the latter case,
   the leaf contents could include a reference to an earlier (complete) leaf
   that this delta applies on top of.

### Leaf Hashing

A Trillian-based transparent Log deals with two distinct leaf hashes, which need
to be defined for each Log application.

The first hash for a leaf in Log is the **Merkle Hash**; this is the hash value
that percolates up the Merkle tree and is therefore incorporated into the
(signed) root hash for the Log; the cryptographic guarantees of the Log's Merkle
tree only apply to data included in the Merkle hash.

The default Merkle hash for a Trillian Log leaf is `SHA-256(0x00 |
leaf.LeafValue)`.

 - For Certificate Transparency, this hash is defined by
   [RFC 6962](https://tools.ietf.org/html/rfc6962#section-3.4) to be the SHA-256
   hash of a zero byte<sup>[1](#note-1)</sup> followed by a TLS-serialized `MerkleTreeLeaf`
   [structure, which includes](https://github.com/google/certificate-transparency/blob/master/docs/images/RFC6962Structures.png)
   the certificate together with a timestamp indicating when it was logged.  A
   Trillian CT Log therefore sets `leaf.LeafValue` to be a TLS-serialized
   `MerkleTreeLeaf`.
 - For Binary Transparency, the data being logged might analogously be a
   structure holding both the (hash of the) binary package and a timestamp
   identifying when it was logged.

A Trillian application may also specify a separate per-leaf **Identity Hash**;
this identifies which leaf values should be considered equivalent, in the sense
that an existing leaf with a given (application-provided) identity hash prevents
Trillian from accepting any new leaves with the same identity hash.

This feature is primarily designed to allow applications to detect (and squash)
semantic duplicates, where two different leaf values actually represent the same
underlying object.

One particular example of this is for applications where it is important to
record the time of logging a thing, together with the thing itself – where a
later attempt to log something that is already logged should be rejected.  This
in turn is related to inclusion promises, see
[below](#inclusion-proofs-vs-promises).

 - For a Trillian-based Certificate Transparency log, the identity hash is a
   hash over the logged certificate itself, without the tree leaf structure that
   includes the log timestamp.  This means that a certificate only gets logged
   once; a second attempt to log the same certificate will have a duplicate
   identity hash even though the Merkle hash is different (because it has a
   different timestamp).
 - A Binary Transparency log might use an identity hash to weed out identical
   package versions, which just covers the binary package blob.
 - A Pastebin log might choose to not set the identity hash, in which case the
   Merkle leaf hash will be used for duplicate detection instead.

### Leaf Size / Accessibility

Another consideration for designing a new transparent Log is the size of each
leaf; Trillian can accommodate fairly large leaves, but would struggle with
storing multi-gigabyte leaf contents<sup>[2](#note-2)</sup>.

In this situation we can appeal to the
[fundamental theorem](https://en.wikipedia.org/wiki/Fundamental_theorem_of_software_engineering)
of security engineering:
> "We can solve any security problem by introducing an extra level of hashing."

A large leaf blob can be stored in a separate (non-transparent) data store
(e.g. a content-addressable store which allows retrieval of the blobs via their
hashes), and the transparent Log carries the cryptographic hashes of the blobs
rather than the blobs themselves.

This approach may also be useful if there are situations where full public
access to the logged data is not appropriate.

 - A Binary Transparency log might include pre-release packages that should not
   (yet) be available to the general public.
 - A Pastebin log might end up containing content that is not legal to
   distribute in some jurisdiction.

## Admission Control

The general idea of transparency can be broken down into two constituent parts,
which we'll call **read-transparency** and **write-transparency**:
 - A read-transparent Log allows any external client to read the contents of the
   Log, and to request cryptographic proofs of the append-only properties
   (inclusion proofs and consistency proofs).
 - A write-transparent Log allows any external client to submit new entries to
   the Log, and receive either an inclusion proof for the entry or a signed
   promise that it will be incorporated in future.

Allowing write-transparency is vital for encouraging an ecosystem to grow up
around the Log(s), and helps to reassure external (read-transparency) users that
no "filtering" has been applied to content before it even reaches the log.

However, it's worth making clear: a write-transparent Log allows arbitrary
people on the Internet to write data into your append-only Log – and that **data
can't be removed<sup>[3](#note-3)</sup> without destroying the Log**.

That's a sufficiently terrifying prospect to motivate having strict **admission
criteria**: checks that submitted content has to pass before being included in
the Log.  (However, this isn't specific to write-transparent Logs; even if
submissions are restricted to a whitelist of clients, the admission criteria
still need to be detailed.)

Examples:

 - For Certificate Transparency, each submission has to take the form of an
   X.509 certificate in ASN.1 DER format, together with a chain of CA
   certificates that leads to one of a set of accepted root certificates.
 - For Binary Transparency, each submission might need to be of the correct
   layout/format to allow installation on a device, and might need to be signed
   by one of a set of acceptable package signing keys.
 - For a Pastebin Log, the admission criteria might merely be a size limit, or a
   UTF-8 encoding correctness check.

These examples illustrate that good admission criteria typically include two key
aspects:

 - Structure: The submissions have to be of the correct format to be of interest
   (so that the submissions match the subject of the Log), and that format has
   to be machine-checkable (but need not be strict – see below).
 - Authentication: The submissions are signed by one of a limited number of
   private keys, so the signature can be checked with the corresponding public
   key.  The Log is thus implicitly:
     - trusting the key holders not to sign anything that might be problematic
       for the Log to host
     - assuming that it will be able to keep up with the numbers of objects
       signed with those private keys.

For our examples:
 - A Certificate Transparency log requires entries to have X.509 structure, and
   trusted-roots authentication.
 - A Binary Transparency log requires entries to have the structure of an
   installable binary package, and vendor-signing key authentication.
 - A Pastebin log has little structural requirement and no authentication
   requirement, and so is much more vulnerable to different attacks (swamping,
   uploading illegal content etc.)

Finally, note that a transparent Log normally acts as an **observatory, not as a
police officer**, in the overall ecosystem.  With this in mind, it's often
sensible for the structure checks on submissions to be lax, so that
technically-invalid objects that are still signed and distributed can be
monitored and attributed.

 - A Certificate Transparency Log may well accept X.509 certificates that fail
   various sorts of validity check, as long as the signature validation is
   correct.  This means that certificates that might be accepted by lax clients
   are still logged, and certificate authorities that issue invalid certificates
   can be held to account more easily.

## Inclusion Proofs vs. Promises

Transparent Logs have two distinct mechanisms for guaranteeing inclusion of a
particular entry in the Log.

 - An **inclusion proof** gives cryptographic proof that a particular Merkle
   tree head includes a given entry: the entry's hash and the proof hashes can be
   combined to recreate the root hash of the tree.
 - An **inclusion promise** is a signed commitment from the Log to incorporate a
   particular entry within a defined time period.  This requires a separate
   subsequent check to confirm that the Log does indeed fulfil this promise.

A related factor is the number of entries that are incorporated into each new
tree head issued by the Log, which we'll call the tree head **batch size**.

 - Logs that issue inclusion promises can batch together multiple submissions
   (e.g. an hour's worth) into a new tree head.  This is more likely to produce
   large batch sizes, but is not guaranteed to (e.g. if submission rates are
   very low).
 - Logs that only issue inclusion proofs are more likely to have smaller batch
   sizes, either because they issue a new tree head per submission (batch size =
   1), or because the amount of batching possible while still responding to
   submissions quickly is small.

The batch size per new tree head may be important for privacy reasons: a small
batch size, particularly a batch of size 1, means that a new signed tree head
correlates directly with a single submission.  This in turn means that a user
requesting proofs to/from that tree head, or gossiping that tree head, is likely
to be interested in that specific entry in the Log.

So, if privacy is a concern:
 - if submission rates are very low, then using inclusion promises with a large
   batch size is more likely to preserve privacy
 - if submission rates are very high, and some latency on submissions is
   acceptable, then just issuing inclusion proofs may be feasible.

A new transparent Log should consider whether it (and the surrounding ecosystem)
requires both inclusion promises and proofs, or just the latter, based on
assessing the concerns above together with other factors:

 - Inclusion promises allow more operational flexibility; a Log can continue to
   issue promises even if its Merkle tree storage is temporarily unavailable,
   and the (potentially) larger batching involved may allow for a more efficient
   implementation.
     - However, issuing a promise that cannot subsequently be fulfilled
       (e.g. because Merkle storage is unavailable for longer than expected, or
       the backlog is too large) may be worse that simply rejecting the write
       operation in the first place.
 - Inclusion promises require separate monitoring to confirm the Log performs
   correctly, making the overall system more complicated.

------

<a name="note-1">1</a>:  Prefix included for
[second-preimage attack resistance](https://en.wikipedia.org/wiki/Merkle_tree#Second_preimage_attack).

<a name="note-2">2</a>: The exact limits depend on the specific storage
implementation in use (e.g. ~10MB for CloudSpanner).

<a name="note-3">3</a>: This isn't strictly true – the Log could replace a
removed leaf with a new leaf type that just holds the Merkle hash value of the
full leaf (and define that the hash value for such a leaf is the
identity).  However, the omission/replacement would be visible to a monitor that
retrieved the log contents.
