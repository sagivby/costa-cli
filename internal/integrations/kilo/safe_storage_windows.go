//go:build windows
// +build windows

package kilo

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Windows-specific safe storage implementation using DPAPI

// LocalState represents the structure of VS Code's Local State file
type LocalState struct {
	OSCrypt struct {
		EncryptedKey string `json:"encrypted_key"`
	} `json:"os_crypt"`
}

// encryptWithSafeStorage encrypts plaintext using Windows DPAPI-based encryption
func encryptWithSafeStorage(plaintext string) ([]byte, error) {
	// Allow tests/CI to override via environment variable
	if v := os.Getenv("COSTA_SAFE_STORAGE_PASSWORD"); v != "" {
		return encryptWithPassword(plaintext, v)
	}

	// Get the encrypted key from Local State
	encryptedKey, err := getWindowsEncryptedKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get encrypted key from Local State: %w", err)
	}

	// Decrypt the key using DPAPI
	masterKey, err := decryptWithDPAPI(encryptedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt master key with DPAPI: %w", err)
	}

	// Use AES-GCM to encrypt the plaintext
	encrypted, err := encryptWithAESGCM(plaintext, masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt with AES-GCM: %w", err)
	}

	// Prefix with "v10" to match VS Code's format
	prefix := []byte("v10")
	return append(prefix, encrypted...), nil
}

// getWindowsEncryptedKey retrieves the encrypted key from VS Code's Local State file
func getWindowsEncryptedKey() ([]byte, error) {
	// Get Local State path
	localStatePath, err := getIDELocalStatePath("vscode")
	if err != nil {
		return nil, err
	}

	// Read the Local State file
	data, err := os.ReadFile(localStatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Local State file: %w", err)
	}

	// Parse JSON
	var localState LocalState
	if err := json.Unmarshal(data, &localState); err != nil {
		return nil, fmt.Errorf("failed to parse Local State JSON: %w", err)
	}

	// Base64 decode the encrypted key
	encryptedKey, err := base64.StdEncoding.DecodeString(localState.OSCrypt.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to base64 decode encrypted key: %w", err)
	}

	// Strip the "DPAPI" prefix if present
	if len(encryptedKey) > 5 && string(encryptedKey[:5]) == "DPAPI" {
		encryptedKey = encryptedKey[5:]
	}

	return encryptedKey, nil
}

// decryptWithDPAPI decrypts data using Windows DPAPI CryptUnprotectData
func decryptWithDPAPI(encryptedData []byte) ([]byte, error) {
	// Load crypt32.dll
	crypt32 := syscall.NewLazyDLL("crypt32.dll")
	procCryptUnprotectData := crypt32.NewProc("CryptUnprotectData")

	// Prepare DATA_BLOB structures
	var dataIn dataBlob
	dataIn.cbData = uint32(len(encryptedData))
	dataIn.pbData = &encryptedData[0]

	var dataOut dataBlob
	var dataDescr uintptr

	// Call CryptUnprotectData
	// Parameters: pDataIn, ppszDataDescr, pOptionalEntropy, pvReserved, pPromptStruct, dwFlags, pDataOut
	ret, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&dataIn)),
		uintptr(unsafe.Pointer(&dataDescr)),
		0, // pOptionalEntropy
		0, // pvReserved
		0, // pPromptStruct
		0, // dwFlags
		uintptr(unsafe.Pointer(&dataOut)),
	)

	if ret == 0 {
		return nil, fmt.Errorf("CryptUnprotectData failed: %v", err)
	}

	// Convert result to Go byte slice
	result := make([]byte, dataOut.cbData)
	copy(result, (*[1 << 30]byte)(unsafe.Pointer(dataOut.pbData))[:dataOut.cbData:dataOut.cbData])

	// Free the memory allocated by CryptUnprotectData
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")
	proc.Call(uintptr(unsafe.Pointer(dataOut.pbData)))

	return result, nil
}

// encryptWithAESGCM encrypts plaintext using AES-GCM
func encryptWithAESGCM(plaintext string, key []byte) ([]byte, error) {
	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Return nonce + ciphertext
	return append(nonce, ciphertext...), nil
}

// encryptWithPassword provides fallback encryption using a password (for testing)
func encryptWithPassword(plaintext, password string) ([]byte, error) {
	// This is a simplified fallback for testing purposes
	// In real usage, we'd use a proper key derivation function
	key := make([]byte, 32)
	copy(key, password)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	iv := make([]byte, aes.BlockSize)
	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	ciphertext := make([]byte, len(padded))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	return ciphertext, nil
}

// pkcs7Pad implements PKCS#7 padding
func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - (len(data) % blockSize)
	if padLen == 0 {
		padLen = blockSize
	}
	padding := make([]byte, padLen)
	for i := range padding {
		padding[i] = byte(padLen)
	}
	return append(data, padding...)
}

// DATA_BLOB structure for Windows API calls
type dataBlob struct {
	cbData uint32
	pbData *byte
}