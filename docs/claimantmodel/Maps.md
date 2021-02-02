# Claimant Model: Maps

This doc presents a model for transparent ecosystems built around maps. This doc requires understanding the core [Claimant Model](CoreModel.md), and it is recommended that [Claimant Model: Logs](Logs.md) is also read first.

## Groundwork: Discoverability

A Claim is discoverable if it can be found by the Claim Verifier without information being passed to them by the Believer.
The Log Claimant Model states that the purpose of Log Transparency is to enable discoverability of Claims.
To understand whether this statement applies to Maps, we need to understand what it is about Logs that enables discoverability.

A Log can be thought of as a Map where the key is the leaf index, and new entries need to be written into the next available leaf.
Maps and Logs both have a single root hash which precisely commits to key/value data, and given this root hash and a key, there is only one verifiable value for the point lookup.
The key difference (pun intended) is that in Maps the writer gets to choose any arbitrary key, where in Logs the key is the next available sequence number.

A Log Checkpoint commits to a range of keys by stating the tree size.
Tree size is the only input required to generate all keys for a log, and it is this property of key enumerability that makes Claim discoverability simple; given a Log Checkpoint, a Verifier can discover all Claims by requesting all entries from [0..treeSize).
Thus providing the Verifier is getting recent Checkpoints and is seeing Checkpoints consistent with the Believer, each and every Claim will be discovered in time.
While simple, this is not efficient, especially for Verifiers interested in a tiny subset of Claims.

Map keys cannot be generated/discovered from a Map Checkpoint, thus Maps do not allow for efficient discoverability through key enumeration.
However, if the expected location for Claims in a Map is precisely understood ahead of time by the Believer and Verifier, Maps can provide Claim discoverability.

## Claim Subject Maps (CSMs)

### New Terms

<dl>
<dt>Claim Subject</dt>
<dd>the entity to which a Claim relates (also referred to as ‘Subject’, for brevity)</dd>

<dt>Mog</dt>
<dd>a Map where the values are Logs</dd>

<dt>Claim Subject Map/Mog (CSM)</dt>
<dd>this is a Map keyed by the Claim Subject. For Mogs, the value is a Log of Claims, for Maps the value is a single Claim. In either case, the Claims keyed under a Subject must relate to the Subject.</dd>
</dl>

### Introduction

CSMs allow for more efficient discoverability of Claims, particularly in ecosystems where there are many independent Claim Verifiers with distinct Claim Subjects.
Claim Verifiers can become somewhat ignorant and selfish, looking after only their own Subjects and trusting others to look after their own.
The trade-off for this is the risk that any Claims placed in the map at locations the Verifier does not expect will not be discovered by the Verifier.
This risk can be eliminated with careful design of Claim Subjects and how they are extracted from Claims.

### Requirements

To use a CSM for discoverability, these properties must hold:
 1. Claim Subjects can be derived from only the Claim. Examples (Claim Subjects in bold):
    * CT: the Claim (cert) contains the **domain(s)** the certificate is for
    * GoLang: the Claim contains the **module name**
 2. Claim Subjects must be keyed only under their canonical form
 3. Claim Verifiers must have a complete set of all Claim Subjects they will verify

#### Canonicalization & Discoverability

A canonical form gives a single location in the map for a Claim Subject. This means that all users looking up Claims related to this see the same value. If there are multiple locations that users can look in, then it’s possible that Believers can be convinced of Claim inclusion in the data structure, but Verifiers are looking elsewhere. This means the Claims are not discoverable, and this undermines the core transparency goal.

The spec for canonicalization should be precise and unambiguous so that it can be implemented in precisely the same way on all clients. Any discrepancy in implementation is a vector for presenting different Claims to different actors. This type of issue is insidious because the data structure verification would pass, giving a false sense of confidence. An attacker could effectively use this to provide a split-view without all the pains required to maintain root commitments that could not be reconciled.

Non-canonical forms are not uniquely a problem for maps. Having ambiguous forms could be a problem for Claim Verifiers in the Log Claimant model too, but there Verifiers have the raw Subject and can canonicalize it, or query for substrings/prefixes, etc.

### Map Functions (deriving Claim Subjects from a Claim)

Using a CSM is simplest in the case where there is a single Subject for every Claim. However, in the general case there are multiple Claim Subjects entailed by a Claim. This apparently minor difference has a large impact on the amount of work required in order to maintain discoverability.

```
mapFn(Claim) []ClaimSubject
```

We’ll use the signature above when describing how Subjects are derived from Claims. For a Believer to trust that a given Claim is discoverable, they must check that the Claim is committed to under all Subjects returned by mapFn.

The reasoning behind this is best understood through example. If we look at the Subjects entailed by a Certificate for `*.foo.example.co.uk` we might derive:
 * `foo.example.co.uk.`
 * `example.co.uk.`
 * `co.uk.`
 * `uk.`
 * `.`

This set of Subjects must have a non-empty intersection with the set of subjects that the Domain Owner is expecting. If this Domain Owner does not have any subdomains and expects to be checking only `example.co.uk.` then a believer convinced only of the inclusion of `foo.example.co.uk.` and no others will have trusted a Claim which cannot be discovered.

This example also highlights that mapFn should not return too many Subjects. While the empty domain `.` may be derived from all Claims, doing so entails a map key which is effectively committing to all entries that would have been in the log. If this is the only mechanism that permits discoverability then a log should have been chosen instead of a map.

### CSM in the Claimant Model
#### Log-Backed Maps & Mogs

If the map is constructed from a Claim Log then we have the following:

<dl>
<dt>Claim<sup>CSM</sup></dt>
<dd><i>"Applying $mapFn to the logged data in $LogCheckpoint results in the map with $rootHash"</i></dd>
<dt>Statement<sup>CSM</sup></dt>
<dd>Map Checkpoint (sometimes SMR, or Signed Map Root)</dd>
<dt>Claimant<sup>CSM</sup></dt>
<dd>CSM Operator</dd>
<dt>Believer<sup>CSM</sup></dt>
<dd>Believer<sup>DOMAIN</sup> and Verifier<sup>DOMAIN</sup></dd>
<dt>Verifier<sup>CSM</sup></dt>
<dd>A Verifier<sup>LOG,DOMAIN</sup> with the power to compute the map</dd>
<dt>Arbiter<sup>CSM</sup></dt>
<dd>CSM Arbiter, possibly the same as Arbiter<sup>LOG</sup></dd>
</dl>

#### General Maps & Mogs
If the map is not constructed from a Claim Log, then the Claim is a lot more nuanced.
The Map Checkpoint needs to explicitly include a Revision identifier that has a total ordering.

General Map Claim:
 * Given revisions N and M (N <= M):
   * Every key/value in N is contained in M

General Mog Claim:
 * Every value in this map is a valid log
 * Given revisions N and M (N <= M):
   * Keyset(N) is a subset of Keyset(M)
   * Every log in N is the prefix of the log under the same key in M
