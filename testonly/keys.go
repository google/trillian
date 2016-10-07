package testonly

// KEYS IN THIS FILE ARE ONLY FOR TESTING. They must not be used by production code.

// DemoPrivateKeyPass is the password for DemoPrivateKey
const DemoPrivateKeyPass string = "towel"

// DemoPrivateKey is the private key itself; must only be used for testing purposes
const DemoPrivateKey string = `
-----BEGIN EC PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-CBC,B71ECAB011EB4E8F

+6cz455aVRHFX5UsxplyGvFXMcmuMH0My/nOWNmYCL+bX2PnHdsv3dRgpgPRHTWt
IPI6kVHv0g2uV5zW8nRqacmikBFA40CIKp0SjRmi1CtfchzuqXQ3q40rFwCjeuiz
t48+aoeFsfU6NnL5sP8mbFlPze+o7lovgAWEqHEcebU=
-----END EC PRIVATE KEY-----`

// DemoPublicKey is the public key that corresponds to DemoPrivateKey.
const DemoPublicKey string = `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEsAVg3YB0tOFf3DdC2YHPL2WiuCNR
1iywqGjjtu2dAdWktWqgRO4NTqPJXUggSQL3nvOupHB4WZFZ4j3QhtmWRg==
-----END PUBLIC KEY-----`
