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
*   **Nuance**: Reality is messy, and "verified" often means reaching a certain level of confidence rather than absolute certainty. The Believer must decide what degree of confidence they require. There is also the nuance that a claim can only be verified with the information available at the time. e.g. "This binary contains no known vulnerabilities" may be true when first checked, but if the knowledge base of vulnerabilities changes, then a future check could fail.
*   **Role of Arbiter**: Less critical for blocking actions, but still useful for penalizing bad Claimants to save others' time.

## Discussion

A Believer can be flexible. They might use "Trust But Verify" for low-stakes actions or highly reputable Claimants, but switch to "Verify Before Use" as the risk increases.

## References

### Core Framework & Theory

*   **[Transparency.dev: Designing a Verifiable System](https://transparency.dev/how-to-design-a-verifiable-system/)**
    *   Description: The official guide for the Claimant Model. Defines the core roles: Claimant, Believer, Verifier, and Arbiter.
*   **[OSFC 2021 Talk: "Designing Transparency Systems using the Claimant Model"](https://osfc.io/2021/talks/designing-transparency-systems-using-the-claimant-model/)**
    *   Description: Presentation by Martin Hutchinson (Google) introducing the model (no video as of 2026).

### Examples of Application

*   **CoSAI: Signing ML Artifacts**
    *   Links:
        *   [CoSAI Signing ML Artifacts Specification](https://github.com/cosai-oasis/ws1-supply-chain/blob/main/signing-ml-artifacts.md)
    *   Description: Applies the Claimant Model to machine learning supply chain security, defining the roles for publishing and verifying ML model signatures.
*   **Google APK & Pixel Binary Transparency**
    *   Links:
        *   [Google APK Binary Transparency Claimant Model](https://developers.google.com/android/binary_transparency/google_apk/overview#claimant_model)
        *   [Pixel Device Binary Transparency Claimant Model](https://developers.google.com/android/binary_transparency/pixel_overview#claimant_model)
    *   Description: Google logs binary hashes to a public log; devices and users verify them to ensure authenticity of the system images and APKs.
*   **Armory Drive Log & Armored Witness**
    *   Links:
        *   [Armory Drive Log Repository](https://github.com/usbarmory/armory-drive-log)
        *   [Armored Witness Claimant Model Mapping](https://github.com/transparency-dev/armored-witness#claimant-model)
    *   Description: Firmware transparency logs and hardware verifier/witness that applies the Claimant Model to verifiable builds and deployments.
*   **Go Checksum Database (SumDB)**
    *   Links:
        *   [Go SumDB Design Doc](https://go.dev/design/25530-sumdb)
        *   [Local Transitive Signing Discussion](Logs.md#transitive-signing)
    *   Description: Logs Go module hashes for verification. Utilizes the Claimant Model to define roles for transitive signing and verification of modules.
*   **Sigstore (Rekor)**
    *   Links:
        *   [Sigstore Homepage](https://www.sigstore.dev/)
    *   Description: Records software signing events in a transparency log. *Note: Sigstore does not have an official Claimant Model mapping document, but its architecture directly implements the Claimant Model pattern.*
*   **Sigsum**
    *   Links:
        *   [Sigsum Claimant Model Documentation](https://git.glasklar.is/sigsum/project/documentation/-/blob/main/claimant.md)
    *   Description: Applies the Claimant Model to make cryptographic key-usage transparent, where key owners (Claimants) log signed checksums, and verifiers check for consistent key usage.

<!-- TODO(mhutchinson): Discuss Closed Loop Systems. -->
