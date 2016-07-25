package testonly

// KEYS IN THIS FILE ARE ONLY FOR TESTING. They must not be used by production code.

// The password for the test key
const CTLogKeyPassword string = "napkin"

// An ECDSA private key for log tests
const CTLogPrivateKeyPEM string = `
-----BEGIN EC PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-CBC,CD876BDE00043553

mvY+JQH/K5NeNba10dtLyvkmVrH+hS9kPkKSk/exHPezCyHF8FytpOMC5sKDj5S4
180O3hcZytZMh3b7lvyimxZ5HmfTm+ZBxAEZCigmb+pBxSzTX7+MK7bew2XZeQdl
p4G1u2PHCzVeyPnRd2XLQ0SBo0T7pKsGVLgae4N45UA=
-----END EC PRIVATE KEY-----
`

// The corresponding public key
const CTLogPublicKeyPEM string = `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEQczNovWyMB74+fAgPAiX60k0M4cO
QYn3hIzwSTVAcrKhHDyT85t4BoXdgl6XzMAVHmF7b6GyKoyyya1hSW4lLg==
-----END PUBLIC KEY-----`

// The log ID we should get from the test keys
const CTLogIDBase64 string = "fMW69rPsQG6J9V1zJeIvei6q+GGYrNxe84y5e0/KaEw="
