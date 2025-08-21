# Claimant Model In-Depth

## Introduction

Transparency logs (tlogs) form the foundation of a number of security architectures.
Ranging from securing TLS, securing the software supply chain, auditing usage of signing keys, etc., tlogs are becoming more prevalant.
Each deployment of tlogs is different, but when looked at from a certain perspective, they can be understood through a common language.
The Claimant Model is the first language designed to communicate about the commonalities all these tlogs share.

The inspiration is [Paxos](https://en.wikipedia.org/wiki/Paxos_(computer_science)), which describes a whole family of situations that solve
_consensus_ in an unreliable network.
The Claimant Model is similar, and describes a whole family of situations that solve _discoverability_ in an ecosystem where trust can't be relied on.

The discoverability property is desirable whenever a situation arises where:
 1. An end-user needs to rely on information provided by a third party
 2. This information is verifiable (aka _falsifiable_)
 3. The end-user cannot verify the information themselves

An ecosystem that has discoverability ensures that any information that is relied upon will ultimately be verified by someone with the ability to do so.
Discoverability is commonly achieved by ensuring that information is first committed to by a tlog, which works as a [verifiable transport layer](https://transparency.dev/articles/logs-a-verifiable-transport-layer/).
For a modern general-purpose implementation of a tlog, we recommend [Tessera](https://github.com/transparency-dev/tessera).

### Why this documentation?

The Claimant Model is very concisely introduced in the [Core Model](./CoreModel.md).
While this concise documentation works well as a refresher for those that already understand the model, it has
empirically proved to be a tough introduction to those new to the domain.
This longer form text on the Claimant Model serves as a more gentle on-ramp to understanding this powerful tool.

This text is broken into sections, but is intended to be read from start to finish.

## Motivation

Prior to the Claimant Model being developed, new transparency projects were designed by copying and modifying
patterns directly from existing deployments, most notably [Certificate Transparency](https://certificate.transparency.dev/) (CT).
While this allows those familiar with CT to reach consensus on a rough design quickly, it has a number of drawbacks:

1. Those that don't already understand CT now have to read [RFC 6962](https://www.rfc-editor.org/rfc/rfc6962) to understand
   the domain that the other designers are drawing from
1. There are still terms in CT that people disagree on the precise definition of (e.g. Auditor and Monitor)
1. CT uses mechanisms such as [SCTs](#scts) that transparency systems should aim to avoid unless strictly necessary
1. Any differences in the roles, interactions, actors, or motivations between CT and the new system being designed can
   have knock-on effects that can undermine the security properties achieved

With this in mind, a clean, role-based model for describing transparency systems was required, and the Claimant Model was born.

## Applicability

The Claimant Model can be used to describe any system where an actor (Believer) performs some trusted action based on some
information (Claim) provided to them by a trusted third-party (Claimant). There is _usually_ an asymmetry, where the
Believer can not verify that the Claim is true, but believes it because they trust the Claimant.

The situation above appears in countless settings in our everyday human lives.
For example if you open a tab at a bar then any drinks are served as a trusted action, relying on you completing your implicit claim that you will pay for the tab before you leave.
Even if you leave a payment card as a requirement for opening the tab, unless some amount is pre-authorized ahead of time, verification by the bank that the card is good for the value of the tab is deferred until after the actions have been performed.

On a technical level, this also happens frequently, often invisibly to the user.
For example, when a Chrome browser (Believer) receives a TLS certificate (Claim) for a domain, it only performs
the connection (trusted action) if the certificate is issued by a Certificate Authority (Claimant) that chains to its
trusted roots.
As another example, a software package manager (Believer) will only install software (trusted action) when the package
(Claim) is signed by the correct package maintainer (Claimant).

Above we state that it must be possible to verify that the Claim is true.
This means that all Claims must be _falsifiable_, i.e. they are able to be proven false.

In both of the examples above, the Believer can verify the cryptographic primitives to operate as the verifier _providing that the Claimant is acting securely and honestly_.
Explicitly, assuming that the CA is the only party with access to their signing key, and that they only issues certificates to legitimate domain owners, and the CA signing cert chains to the trusted root set, then verification can be performed by Chrome.
Similarly, assuming that the package maintainer never loses control of their key material and that they only ever sign good packages, then the package manager can verify the signature.

These assumptions can be weakened, but this will require another party to perform additional [verification](#verification).

### Exceptions

There are some situations that the Claimant Model _could_ be used to describe, but taking the time to do so would be a poor
return on investment, or more simply, just confuse a situation that can be easily stated in simpler terms.

- Believer can trivially verify the Claim: trust in the Claimant is a non-factor in this case. For example, if a web
  service exists that can return the prime factors of some large number, the Believer may not have been able to perform
  this factorization itself, but it can trivially multiply the factors together to confirm the result.
- Believer will trust Claims from anyone: if the identity of the Claimant is not a factor in the Believer's decision to
  perform some action, then the Claimant Model provides little benefit. Such situtations are unfortunately
  still common, e.g. installing unsigned software/firmware.

## Trust

Any ecosystem where the Claimant Model is [applicable](#applicability) has potential for exploit.
Specifically, a false Claim made under the identity of the Claimant would be believed by a Believer, and they would
perform some trusted action they would not have made given a true Claim.
This potential for exploit exists because there is an asymmetry between who believes a claim and who can verify it.
The Claimant Model shines a light on this asymmetry and once a system has been described it should be clear how this tension will be resolved.

### Motivations for Betraying Trust

There are many reasons that a Claimant may make incorrect Claims, and these are worth considering along with a threat model.
As a non-exhaustive list of reasons that a Claimant may make a false claim:
- Accidental:
  - Bugs in code that the Claimant runs
  - Human error in following process
  - Massively unlikely, but [possible](https://www.agwa.name/blog/post/how_ct_logs_fail) random uncontrollable events
- Greed: the Claimant decided that abusing their trust was financially more beneficial than continuing operating in
  a trustworthy manner (e.g. exit scams in the cryptocurrency world)
- Compromise:
  - Insider Threat: a single agent at the company has motivation to exploit trust
  - Leverage: a third-party could coerce the Claimant into betraying trust
  - Compromise: actors that breach the Claimant security mechanisms could issue false claims directly by stealing signing keys,
    or change code or playbooks in such a way that the Claimant themselves issues false claims

There are also pressures in the opposite direction, i.e. motivations to *never* issue a bad claim.
Consider the situation where each Claim is provided to a single Believer who would never realize the Claim was bad, and then
compare this to a situation where every Claim is made in a publicly visible forum.
In the first situation, maybe a Claimant could be coerced into making a bad Claim "just this one time", where in the second
case it would be discovered and a public investigation launched.
This is the motivation for [transparency logs](https://transparency.dev).

## Verification

Given enough time, potential motivation, and/or bad luck, any Claimant could issue a false Claim.
The question is how to make a system with enough security given this fact.

Earlier we established that Claims must be falsifiable, but that in general the Believer will not be able to perform this verification.
Thus we introduce the role of the _Verifier_, which will be able to verify all Claims in a system.

For a Verifier to be able to check all Claims, it must first be able to find them.
This is the principle of **discoverability**: ensuring that any information relied upon by a Believer will ultimately be seen and checked by a Verifier.
Discoverability is typically achieved by requiring all Claims to be published in a public, append-only [transparency log](https://transparency.dev) (tlog).

By forcing all Claims to be logged, the system ensures that a Claimant cannot make a private Claim to a Believer that no one else can see.
Any Believer, before acting on a Claim, can require proof that the Claim has been included in the log.
Verifiers can then monitor the log's contents, inspect every Claim that has been made, and check its validity.
If a false Claim is discovered, the Verifier can then take action, such as notifying an Arbiter (e.g. notifying an authority, or making the misbehavior public).
This public auditability is what provides the security guarantee; Claimants are discouraged from making false claims because they know they will be discovered.

## More Precision

In the interests of making the above as accessible to newcomers as possible, some amount of precision has
been sacrificed.
If you're still following along at this point, then it seems like a good time to discuss some topics that
aren't necessarily difficult, but cross-cut the model in ways that make it too overwhelming to introduce earlier.

### Statements vs Claims

We have used the term Claim until this point to mean two related, but conceptually distinct things:
- A falsifiable logical statement
- Data

The first of these is the correct usage of Claim: a falsifiable logical statement made by the Claimant.
The second of these is often referred to as the Claim at high levels, but where precision is required we should refer
to this as the `Statement`.

A Statement must be issued by the Claimant and it must be possible to construct the Claim from it.
In practical scalable systems, Statements are determined to have been issued by a Claimant by the presence of
a digital signature over the Statement.

For example, in CT the Claim is the logical statement "I am authorized to certify $pubKey for $domain" made by the CA, and the Statement is an X.509 certificate containing $pubKey and $domain, and signed by the CA.
Given an X.509 certificate it would be possible to construct the Claim from it by extracting the relevant fields.

### Composite Claims

Thus far we've referred to the Believer taking action based on a single Claim.
This isn't incorrect, but at times it is useful to consider a single Claim as multiple sub-claims.
In particular, this is useful when a claim has multiple sub-claims that can be verified by different actors.

When a Claim is composed of multiple sub-claims, the Believer's trust decision may depend on the validity of all of them.
Each sub-claim can have its own Claimant and Verifier. This allows for a separation of concerns and a more robust trust model.

For example, consider a software supply chain security scenario: a user (Believer) wants to install a software package. The high-level Claim is "this package is safe to install". This can be broken down into several sub-claims:

*   **Sub-claim 1: Source Integrity.** "This binary package was built from the source code at commit `abc` in repository `XYZ`."
    *   **Claimant:** A trusted, reproducible build service.
    *   **Verifier:** Anyone who can re-run the build from the same source and verify that the output is identical.
*   **Sub-claim 2: Source Security.** "The source code at commit `abc` has passed all security scans and has no critical vulnerabilities."
    *   **Claimant:** An automated security scanning tool or a security audit firm.
    *   **Verifier:** A security team that can review the scan results or the audit report.
*   **Sub-claim 3: Author Identity.** "The source code was authored and signed by a trusted developer."
    *   **Claimant:** The developer.
    *   **Verifier:** The developer themselves (to ensure their key wasn't used without their knowledge), or a system that tracks developer identities.

The package manager (Believer) will only proceed with the installation if all these sub-claims are present and appear valid. Different Verifiers, each with their own expertise, are responsible for checking the different facets of the overall Claim. This composite approach provides a much stronger security guarantee than a single, monolithic claim.

### Actors and Roles

In the Claimant Model, we distinguish between a _Role_ and an _Actor_.

*   A **Role** is an abstract description of a function or responsibility within the system. The primary Roles we have discussed are Claimant, Believer, Verifier, and Arbiter.
*   An **Actor** is a concrete entity, such as a person, a company, or a software system, that performs one or more Roles.

It is tempting to think of a one-to-one mapping, where each Actor performs exactly one Role. However, the relationship is often more complex and is best considered a many-to-many mapping. Understanding this distinction is key to accurately modeling real-world systems.

This means:
*   **One Actor can play multiple Roles.** This is common in systems where an entity has multiple responsibilities. For example, in [signature transparency](https://www.sigstore.dev/) use-cases, a software developer (`Actor`) is the Claimant when they sign their code. They are also the primary Verifier, as they are the one best positioned to check a transparency log to ensure that their signing key has not been used to sign anything without their knowledge.

*   **One Role can be fulfilled by multiple Actors.** This often happens when a responsibility is distributed. For instance, the Verifier Role is frequently fulfilled by a diverse set of Actors. In Certificate Transparency, browser vendors, CAs, and independent researchers all act as Verifiers. In systems with [composite claims](#composite-claims), different Actors may verify different sub-claims, yet they all share the single Role of Verifier for the overall system.

## Other Reading

- [Transparency: An Implementer's Story](https://transparency.dev/articles/transparency-an-implementers-story/) provides a short,
  narrative-driven journey that would be useful to those understanding how to upgrade their existing security posture to include logs.

## Appendices

While writing this documentation the need for other supporting documentation is frequently requested in the comments.
These topics are important but are outside the scope of what this documentation is aiming to provide.
As a pragmatic compromise, such topics will be _very briefly_ summarized here along with a feature request bug tracker to write up the topic in more detail.
Once such documentation is complete, this appendix stub should be removed and all hyperlinks updated.

### SCTs

Signed Certificate Timestamps (SCTs) are a design choice from Certificate Transparency that is *not recommended* as a default choice in new transparency applications.
This is not to say that this pattern should never be used, but alternatives should be strongly considered first.

When an item is added to a log, it is usually an asynchronous operation, i.e. there is a delay between the item being sent to the log and the item being provably committed to by the Merkle structure of the log.
It is standard practice that logs publish a Maximum Merge Delay (MMD) for what this delay can be.
In the case of CT this is 24h.

A long merge delay makes it easier for log operators to operate the logs within [SLA](https://en.wikipedia.org/wiki/Service-level_agreement), but this comes at the expense of Believers not being able to rely on proving inclusion of any claim that is fresher than the MMD in the worst case.

SCTs are a promise that is issued when a claim is sent to the log that claims that the log operator promises to include the claim by a given integration time.
This promise is signed by the log and returned to the user that logged the claim.
If you're getting that feeling that this itself now feels like a Claim, then bingo:

<dl>
<dt>Claim<sup>PROMISE</sup></dt>
<dd><i>${claim} will be committed to by the Merkle structure of this log by $integrationTime</i></dd>
<dt>Claimant<sup>PROMISE</sup></dt>
<dd>Log Operator</dd>
<dt>Believer<sup>PROMISE</sup></dt>
<dd>Believer<sup>LOG</sup></dd>
<dt>Verifier<sup>PROMISE</sup></dt>
<dd>Anyone with the promise at $integrationTime</dd>
<dt>Arbiter<sup>PROMISE</sup></dt>
<dd>Arbiter<sup>LOG</sup></dd>
</dl>

Using the Claimant Model to describe promises succinctly shows the inherent problem with them; promises need to be verified.
Anyone _can_ verify an inclusion promise, but making it so that everyone will verify any promise they have seen is generally not feasible.
For example, in CT this would require that every browser that has been shown a certificate containing one or more SCTs would verify the inclusion in all logs that had issued SCTs after the MMD had passed.
This would place a significant burden on clients (impacting network, storage, and battery) and require a mechanism for reporting failures.
This would also have privacy implications, as the user would be revealing informations about their browsing history to the log operators by the inclusion proofs they check.

The other option is that SCTs are made discoverable to a party of dedicated verifiers who can report non-fulfilled promises to the arbiter.
This discoverability problem would require a log for SCTs, which then appears to be precisely the same problem as logging certificates in the first place!

#### Alternatives to SCTs

The simplest solution is to reduce or eliminate the long merge delay. If a log can offer an MMD of a few seconds, a user can simply wait for the inclusion proof to be available before using the Claim. This shifts the burden to log operators, who must maintain highly available and performant infrastructure, but it dramatically simplifies the client-side logic and verification story.

