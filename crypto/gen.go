package crypto

//go:generate mockgen -package crypto -destination mock_signer.go crypto Signer
//go:generate mockgen -self_package github.com/google/trillian/crypto -package crypto -destination mock_key_manager.go github.com/google/trillian/crypto KeyManager
