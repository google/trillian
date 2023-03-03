{{ $logClaimant := .Log.Claimant }}
```mermaid
sequenceDiagram
{{- range .Actors}}
    actor {{.}}
{{- end}}
    {{.Domain.Claimant}}->>{{.Log.Claimant}}: Add new {{.Domain.Statement}}
    {{.Log.Claimant}}->>{{.Log.Claimant}}: Integrate {{.Domain.Statement}}s and issue {{.Log.Statement}}
    {{.Log.Claimant}}->>{{.Domain.Claimant}}: {{.Log.Statement}} and inclusion proof
    {{.Domain.Claimant}}->>{{.Domain.Believer}}: {{.Domain.Statement}} with proof bundle
    {{.Domain.Believer}}->>{{.Domain.Believer}}: Verify bundle and TODO(TRUSTED ACTION)
    loop Periodic append-only Verification
        Witness->>{{$logClaimant}}: Fetch merkle data
        Witness->>Witness: Verify append-only
    end
    loop Periodic {{.Domain.Statement}} Verification
{{- range $verifier, $claim := .Domain.VerifierList}}
        {{$verifier}}->>{{$logClaimant}}: Get all entries
        {{$verifier}}->>{{$verifier}}: Verify: {{$claim}}
{{- end}}
    end
```