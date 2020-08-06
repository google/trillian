# Claimant Model

This paper introduces an intuitive, standalone model for a common scenario where one party needs to perform an action which is reasonable only if a claim made by another party is true. The Claimant Model allows formally verifiable constructs to be bound to a messy reality.

## Model

*This model describes roles, not actors. In a concrete deployment of this model, the same actor may play multiple roles, or multiple actors may need to work together to cover a role.*

There is a **Claimant** that makes a **Claim** that is relied upon by a **Believer** as a precondition to take an action they would not have taken if the claim was false. Claims are represented by signed **Statements**. The veracity of a Claim can be verified by a **Claim Verifier**, who will notify a **Claim Arbiter** of any false Claims.

## Example: Certification Claims
* Claim<sup>CERT</sup>: (I, ${CA}, am authorized to certify $pubKey for $domain)
* Statement<sup>CERT</sup>: X.509 Certificate
* Claimant<sup>CERT</sup>: Certificate Authority
* Believer<sup>CERT</sup>: User Agent (Browser)
* Verifier<sup>CERT</sup>: Domain Owner
* Arbiter<sup>CERT</sup>: User Agent Vendor

The CA creates and signs a Certificate. The UA receives this cert when attempting to establish a secure connection to a domain, and proceeds with the connection only if it was signed by a CA that the Browser Vendor has blessed. In the event of a CA issuing bad certificates, the Browser Vendor can protect their users by removing the CA from the blessed set. The only party which can verify that a certificate was issued under authorization is the Domain Owner.

The Browser has sufficient trust in the CA to employ a strategy of [Trust But Verify](#trust-but-verify), however this model alone does not provide a mechanism to ensure that any Statement<sup>CERT</sup> relied on by Believer<sup>CERT</sup> can be discovered by Verifier<sup>CERT</sup>. This is why Certificate Transparency was created.

<!-- TODO(mhutchinson): Using Logs to provide this discoverability is described in [Claimant Model: Logs](Logs.md). -->
<!-- TODO(mhutchinson): Discuss Closed Loop Systems below and link to this. -->

# Claim Requirements
* The Statement for any claim must be signed by the Claimant to ensure that it is immutable and non-repudiable.
* It must be possible to generate the Claim from the Statement without any additional context.
* Claims must be falsifiable.
  
## Compound Claims
When the Claim is composed of multiple subclaims, the Verifier role must be covered by actors that can verify all components of the Claim. For example, imagine a Claimant<sup>PRIME</sup> that provides Believer<sup>PRIME</sup> with an integer covered by Claim<sup>PRIME</sup>: "This is prime, and is uniquely issued to $believerID". This is a compound Claim: verification would need to confirm 1) the primality of the integer, and 2) that no other Believer had been issued the same value.

# Believer Strategies
## Trust But Verify
If the Believer has sufficient trust to believe the Claimant for some action, Trust But Verify allows the Believer to take the Claim on face value, and perform the action before the Claim Verifier verifies this Claim. This system requires all Claims trusted by the Believer to be discovered by a Verifier, and this is what is meant by transparency. Such systems generally include a Claim Arbiter that closes the feedback loop by notifying Believers of bad Claimants, and of new Claimants that can be trusted.

## Verify Before Use
If the Believer does not have sufficient trust in the Claimant to perform an action without verifying the claim, then the Believer must wait until the claim is verified. It is attractive to imagine that the veracity of a Claim starts UNKNOWN and then is verified to be either TRUE or FALSE, and this will hold for all time. Reality isn't as kind as this, and the veracity of claims is best considered a degree of confidence that can be very volatile as new data points are brought to light. Believers need to determine what degree of confidence they require before proceeding with an action. The role of the Arbiter is less critical in this strategy, but they can still be included to prevent bad Claimants from wasting other actor's time.

## Discussion
A Believer can be flexible about which strategy they employ. For example: Trust But Verify might be reasonable for a low-stakes action based on a Claim from a Claimant with a good reputation; allowing the action to be taken without blocking. As the potential loss involved in the action increases, the Believer will demand increased trust in the Claimant and/or a higher degree of verification. Some systems may also provide other incentives, such as some promise of compensation from the arbitration process (e.g. FSCS).

<!-- TODO(mhutchinson): Discuss Closed Loop Systems. -->
