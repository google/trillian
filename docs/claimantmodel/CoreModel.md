# Claimant Model

The Claimant Model provides a framework for analyzing trust in systems. It helps map formal cryptographic or verifiable constructs to real-world scenarios where one party needs to rely on information provided by another.

## Core Roles

*Note: This model describes **roles**, not specific actors. In practice, a single actor might play multiple roles, or multiple actors might share a single role.*

The model defines five key roles:

*   **Claimant**: The party that makes a **Claim**.
*   **Claim**: A falsifiable logical statement made by the Claimant.
*   **Statement**: The concrete data (usually signed) that represents the Claim.
*   **Believer**: The party that relies on the Claim to take an action. They would not take this action if the Claim were false.
*   **Claim Verifier**: The party that checks whether the Claim is actually true.
*   **Claim Arbiter**: The party that handles disputes and takes action if a false Claim is discovered.

## Example: Certificate Transparency

To see the model in action, consider how it applies to web security and Certificate Authorities (CAs).

*   **Background**: A CA signs a digital certificate for a domain. A web browser trusts this certificate to establish a secure connection.
*   **The Tension**: A CA should only issue a certificate if the domain owner requests it. If a CA issues a certificate maliciously or by mistake, the browser relies on a false claim.

Here is how we map this to the Claimant Model roles for this specific domain:

*   **Cert Claim**: *"I, [CA Name], am authorized to certify [Public Key] for [Domain]."*
*   **Cert Statement**: The X.509 Certificate itself.
*   **Cert Claimant**: The Certificate Authority.
*   **Cert Believer**: The Web Browser (which connects based on the cert).
*   **Cert Verifier**: The Domain Owner (the only one who knows if they requested the cert).
*   **Cert Arbiter**: The Browser Vendor (who can remove the CA from the trusted set if they misbehave).

> [!NOTE]
> **Notation Note:** In complex systems, we often apply the Claimant Model at multiple layers (e.g., the certificate layer and the log layer). To distinguish roles at different layers, we use descriptive prefixes (like **Cert** Claimant or **Log** Claimant). An alternative way of formatting this in older versions of this documentation was using superscripts (e.g., Claimant<sup>CERT</sup>), but this was deprecated in favor of straight text for accessibility and readability.

In this example, the Browser has sufficient trust in the CA to employ a "**Trust But Verify**" strategy. However, the model above doesn't guarantee that the **Cert Verifier** (Domain Owner) will ever *see* the certificate that the **Cert Believer** relied on. This gap is why Certificate Transparency was created—to make all issued certificates discoverable.

(For details on how logs fill this gap, see [Claimant Model: Logs](Logs.md)).

## Claim Requirements

For the model to work effectively, Claims and Statements must meet certain criteria:
*   **Signed Statements**: The Statement for any claim must be signed by the Claimant to ensure it is immutable and non-repudiable.
*   **Self-Contained**: It must be possible to extract the Claim from the Statement without needing additional context.
*   **Falsifiable**: Claims must be able to be proven false.

### Compound Claims

Sometimes a Claim is composed of multiple sub-claims. In these cases, the Verifier role must be covered by actors that can verify all components of the Claim.

*Example:* Imagine a **Prime Claimant** that provides a **Prime Believer** with an integer covered by a **Prime Claim**: *"This integer is prime, and it is uniquely issued to [Believer ID]."* This is a compound claim because verification requires checking two different things:
1.  Is the integer actually prime?
2.  Has this integer been issued to anyone else?

## Believer Strategies

How a Believer acts on a Claim depends on their level of trust. There are two primary strategies:

### 1. Trust But Verify
If the Believer has sufficient trust in the Claimant, they may take the Claim at face value and act on it *before* it is verified. This is how Certificate Transparency works.
*   **Requirement**: This strategy requires all Claims to be discoverable by Verifiers so that accountability is possible after the fact.
*   **Role of Arbiter**: Critical here to penalize bad Claimants and protect future Believers.

### 2. Verify Before Use
If the Believer does not have sufficient trust, they must wait until the Claim is verified before acting.
*   **Nuance**: Reality is messy, and "verified" often means reaching a certain level of confidence rather than absolute certainty. The Believer must decide what degree of confidence they require.
*   **Role of Arbiter**: Less critical for blocking actions, but still useful for penalizing bad Claimants to save others' time.

## Discussion

A Believer can be flexible. They might use "Trust But Verify" for low-stakes actions or highly reputable Claimants, but switch to "Verify Before Use" as the risk increases.

<!-- TODO(mhutchinson): Discuss Closed Loop Systems. -->
