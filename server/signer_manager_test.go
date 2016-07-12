package server

import (
	"errors"
	"io"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/storage"
	"github.com/stretchr/testify/mock"
)

func TestSignerManagerNothingToDo(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockKeyManager := new(crypto.MockKeyManager)

	sm := NewSignerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{}, createTestContext(mockStorageProviderForSequencer(mockStorage)))

	mockStorage.AssertExpectations(t)
	mockKeyManager.AssertExpectations(t)
}

func TestSignerManagerBeginFails(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)
	mockStorage.On("Begin").Return(mockTx, errors.New("TX"))
	mockKeyManager := new(crypto.MockKeyManager)

	sm := NewSignerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{logID1}, createTestContext(mockStorageProviderForSequencer(mockStorage)))

	mockTx.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
	mockKeyManager.AssertExpectations(t)
}

func TestSignerManagerGetLatestRootFails(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)
	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("LatestSignedLogRoot").Return(testRoot0, errors.New("getroot"))
	mockTx.On("Rollback").Return(nil)
	mockKeyManager := new(crypto.MockKeyManager)

	sm := NewSignerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{logID1}, createTestContext(mockStorageProviderForSequencer(mockStorage)))

	mockTx.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
	mockKeyManager.AssertExpectations(t)
}

func TestSignerManagerSignRootFails(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)
	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("LatestSignedLogRoot").Return(testRoot0, nil)
	mockTx.On("Rollback").Return(nil)
	mockKeyManager := new(crypto.MockKeyManager)
	mockSigner := new(crypto.MockSigner)
	mockKeyManager.On("Signer").Return(mockSigner, errors.New("keymanager"))

	sm := NewSignerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{logID1}, createTestContext(mockStorageProviderForSequencer(mockStorage)))

	mockTx.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
	mockKeyManager.AssertExpectations(t)
	mockSigner.AssertExpectations(t)
}

func TestSignerManagerWriteRootFails(t *testing.T) {
	hasher := trillian.NewSHA256()

	expectedRoot := trillian.SignedLogRoot{
		TimestampNanos: 1467121212000000045,
		RootHash:       []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55},
		TreeSize:       0,
		Signature:      &trillian.DigitallySigned{Signature: []byte("signed")},
		LogId:          logID1.LogID,
		TreeRevision:   1}

	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)
	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("LatestSignedLogRoot").Return(testRoot0, nil)
	mockTx.On("StoreSignedLogRoot", expectedRoot).Return(errors.New("writeroot"))
	mockTx.On("Rollback").Return(nil)
	mockKeyManager := new(crypto.MockKeyManager)
	mockSigner := new(crypto.MockSigner)
	mockSigner.On("Sign", mock.MatchedBy(
		func(other io.Reader) bool {
			return true
		}), []byte{0x5d, 0xf6, 0xe0, 0xe2, 0x76, 0x13, 0x59, 0xd3, 0xa, 0x82, 0x75, 0x5, 0x8e, 0x29, 0x9f, 0xcc, 0x3, 0x81, 0x53, 0x45, 0x45, 0xf5, 0x5c, 0xf4, 0x3e, 0x41, 0x98, 0x3f, 0x5d, 0x4c, 0x94, 0x56}, hasher).Return([]byte("signed"), nil)
	mockKeyManager.On("Signer").Return(mockSigner, nil)

	sm := NewSignerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{logID1}, createTestContext(mockStorageProviderForSequencer(mockStorage)))

	mockTx.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
	mockKeyManager.AssertExpectations(t)
	mockSigner.AssertExpectations(t)
}

func TestSignerManager(t *testing.T) {
	hasher := trillian.NewSHA256()

	expectedRoot := trillian.SignedLogRoot{
		TimestampNanos: 1467121212000000045,
		RootHash:       []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55},
		TreeSize:       0,
		Signature:      &trillian.DigitallySigned{Signature: []byte("signed")},
		LogId:          logID1.LogID,
		TreeRevision:   1}

	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)
	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("LatestSignedLogRoot").Return(testRoot0, nil)
	mockTx.On("StoreSignedLogRoot", expectedRoot).Return(nil)
	mockTx.On("Commit").Return(nil)
	mockKeyManager := new(crypto.MockKeyManager)
	mockSigner := new(crypto.MockSigner)
	mockSigner.On("Sign", mock.MatchedBy(
		func(other io.Reader) bool {
			return true
		}), []byte{0x5d, 0xf6, 0xe0, 0xe2, 0x76, 0x13, 0x59, 0xd3, 0xa, 0x82, 0x75, 0x5, 0x8e, 0x29, 0x9f, 0xcc, 0x3, 0x81, 0x53, 0x45, 0x45, 0xf5, 0x5c, 0xf4, 0x3e, 0x41, 0x98, 0x3f, 0x5d, 0x4c, 0x94, 0x56}, hasher).Return([]byte("signed"), nil)
	mockKeyManager.On("Signer").Return(mockSigner, nil)

	sm := NewSignerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{logID1}, createTestContext(mockStorageProviderForSequencer(mockStorage)))

	mockTx.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
	mockKeyManager.AssertExpectations(t)
	mockSigner.AssertExpectations(t)
}
