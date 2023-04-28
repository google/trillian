# Claimant Model In-Depth

## Introduction

The Claimant Model is very concisely introduced in the [Core Model](./CoreModel.md).
While this concise documentation works well as a refresher for those that already understand the model, it has
empirically proved to be a tough introduction to those new to the domain.
This longer form text on the Claimant Model serves as a more gentle on-ramp to understanding this powerful tool.

This text is broken into sections, but is intended to be read from start to finish.

## Motivation

Prior to the Claimant Model being developed, new transparency projects were designed by copying and modifying
patterns from existing deployments, most notably [Certificate Transparency](https://certificate.transparency.dev/) (CT).
While this allows those familiar with CT to reach consensus on a rough design quickly, it has a number of drawbacks:

1. Those that don't already understand CT now have to read [RFC 6962](https://www.rfc-editor.org/rfc/rfc6962) to understand
   the domain that the other designers are drawing from
1. There are still terms in CT that people disagree on the precise definition of (e.g. Auditor and Monitor)
1. CT uses mechanisms such as SCTs that transparency systems should aim to avoid unless strictly necessary
1. Any differences in the roles, interactions, actors, or motivations between CT and the new system being designed can
   have knock-on effects that can undermine the security properties achieved

With this in mind, a clean, role-based model for describing transparency systems was required, and the Claimant Model was born.

## Applicability

The Claimant Model can be used to describe any system where an actor (`Believer`) performs some trusted action based on some
information (`Claim`) provided to them by a trusted third-party (`Claimant`). There is _usually_ an asymmetry, where the
Believer can not verify that the Claim is true, but believes it because they trust the Claimant.

The situation above appears in countless settings in our everyday human lives.
On a technical level, this also happens frequently, often invisibly to the user.
For example, when a Chrome browser (`Believer`) receives a TLS certificate (`Claim`) for a domain, it only performs
the connection (trusted action) if the certificate is issued by a Certificate Authority (`Claimant`) that chains to its
trusted roots.
As another example, a software package manager (`Believer`) will only install software (trusted action) when the package
(`Claim`) is signed by the correct package maintainer (`Claimant`).

Above we state that it must be possible to verify that the Claim is true.
This means that all Claims must be _falsifiable_, i.e. they are able to be proven false.
We'll discuss _who_ can falsify Claims later.

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

Earlier we established that Claims must be falsifiable, but that in general the Believer will not be able
to perform this verification.
Thus we introduce the role of the `Verifier`, which will be able to verify all Claims in a system.

TODO: link to sidebar discussing roles vs actors: the Verifier role does not imply there is a single actor that must
be able to verify all the claims.

## More Precision

In the interests of making the above as accessible to newcomers as possible, some amount of precision has
been sacrificed.
If you're still following along at this point, then it seems like a good time to discuss some topics that
aren't necessarily difficult, but cross-cut the model in ways that make it too overwhelming to introduce earlier.

### Statements vs Claims

We have used the term Claim until this point to mean two related, but conceptually distinct things:
- A falsifiable logical statement
- Data

The first of these is the correct usage of `Claim`: a falsifiable logical statement made by the Claimant.
The second of these is often referred to as the Claim at high levels, but where precision is required we should refer
to this as the `Statement`.

A Statement must be issued by the Claimant and it must be possible to construct the Claim from it.
In practical scalable systems, Statements are determined to have been issued by a Claimant by the presence of
a digital signature over the Statement.

### Composite Claims

Thus far we've referred to the Believer taking action based on a single Claim.
This isn't incorrect, but at times it is useful to consider a single Claim as multiple sub-claims.
In particular, this is useful when a claim has multiple sub-claims that can be verified by different actors.

TODO: example

## Other Reading

- [Transparency: An Implementer's Story](https://transparency.dev/articles/transparency-an-implementers-story/) provides a short,
  narrative-driven journey that would be useful to those understanding how to upgrade their existing security posture to include logs.
