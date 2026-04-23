# Claimant Model: Maps

This document applies the Claimant Model to transparent ecosystems built around **Verifiable Maps**. In this context, these maps take the form of **Claim Subject Maps (CSMs)**, where the keys are the "Claim Subjects" (the specific entity a claim is about).

The intended user of this kind of map is the **Domain Verifier**. While logs provide discoverability by allowing a verifier to read all entries sequentially, maps allow a verifier to efficiently look up only the specific subjects they care about.

In fact, a verifiable map built on top of an append-only log is the primary expected deployment model for these systems, combining the auditability of logs with the efficiency of maps.

This document requires an understanding of the core [Claimant Model](CoreModel.md) and is best read after [Claimant Model: Logs](Logs.md).

## Groundwork: Discoverability

A Claim is discoverable if the Claim Verifier can find it without relying on the Believer to pass it to them.

The purpose of Log Transparency is to enable discoverability of Claims. To understand whether this applies to Maps, we need to look at how they differ from Logs:

*   **Logs**: Can be thought of as a Map where the key is the leaf index. The writer cannot choose the key; it is the next available sequence number. This makes enumeration easy: given a tree size, a Verifier can discover all Claims by requesting all entries from `0` to `treeSize`. This makes discoverability simple but inefficient for large datasets if you only care about a few entries.
*   **Maps**: The writer gets to choose arbitrary keys. A Verifier cannot guess all keys from a Map Checkpoint alone, so they cannot discover all claims by enumeration.

However, if the expected location for Claims in a Map is precisely understood ahead of time by both the Believer and Verifier, Maps can provide efficient discoverability.

## Claim Subject Maps (CSMs)

### New Terms

*   **Claim Subject**: The entity to which a Claim relates (e.g., a domain name in a certificate).
*   **Mog**: A Map where the values are Logs (Map + Log).
*   **Claim Subject Map/Mog (CSM)**: A Map keyed by the Claim Subject. The value is either a single Claim (for Maps) or a Log of Claims (for Mogs). Crucially, the claims keyed under a Subject must relate to that Subject.

### Introduction

CSMs allow for more efficient discoverability of Claims, particularly when there are many independent Claim Verifiers interested in distinct Subjects (e.g., different domain owners).

Verifiers can look after only their own Subjects, trusting others to do the same. The risk is that any Claims placed at locations the Verifier does not expect will not be discovered. This risk can be eliminated with careful design of Claim Subjects.

### Requirements

To use a CSM for discoverability, these properties must hold:
1.  **Derivable Subjects**: Claim Subjects must be derived from the Claim itself (e.g., extracting the domain from a cert).
2.  **Canonical Forms**: Claim Subjects must be keyed only under their canonical form to avoid split-view attacks where users look in different places for the same thing.
3.  **Known Subjects**: Claim Verifiers must know the complete set of Subjects they intend to verify.

### Map Functions

We use a map function to derive Subjects from a Claim: `mapFn(Claim) []ClaimSubject`.

For a Believer to trust that a Claim is discoverable, they must check that the Claim is committed to under *all* Subjects returned by `mapFn`. For example, a certificate for `*.foo.example.co.uk` might entail subjects like `foo.example.co.uk`, `example.co.uk`, etc. The Verifier must check the locations they expect.

## CSM in the Claimant Model

If the map is constructed from a Claim Log (a Log-Backed Map), we can model it as:

*   **CSM Claim**: *"Applying the map function to the logged data committed to by [Log Checkpoint] results in the map with [Root Hash]."*
*   **CSM Statement**: Map Checkpoint (sometimes called Signed Map Root or SMR).
*   **CSM Claimant**: The CSM Operator.
*   **CSM Believer**: Both the **Domain Believer** and the **Domain Verifier**.
*   **CSM Verifier**: A Verifier with the power to compute the map from the log (verifying the map construction).
*   **CSM Arbiter**: The CSM Arbiter (likely the same as the Log Arbiter).

### Optimization: Verifiable Index (VIndex)

When deploying a map built on top of a log, a potential inefficiency arises if the map duplicates the full data of the claims already stored in the log.

The [Verifiable Index (VIndex)](https://github.com/transparency-dev/incubator/tree/main/vindex) is an optimization designed to solve this. Instead of repeating the full log entries or claims in the map values, the VIndex map simply stores **pointers** (the sequence numbers or indices) back to the location of the claim in the original log. This keeps the map small and efficient while maintaining full discoverability and verifiability.
