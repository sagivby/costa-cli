//go:build !windows
// +build !windows

package kilo

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"

	"github.com/zalando/go-keyring"
)

// encryptWithSafeStorage encrypts plaintext using platform-specific safe storage
func encryptWithSafeStorage(plaintext string) ([]byte, error) {
	password, err := getSafeStoragePassword()
	if err != nil {
		return nil, err
	}

	key := pbkdf2SHA1([]byte(password), []byte("saltysalt"), 1003, 16)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	iv := bytes.Repeat([]byte(" "), aes.BlockSize)
	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	ciphertext := make([]byte, len(padded))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	prefix := []byte("v10")
	return append(prefix, ciphertext...), nil
}

// getSafeStoragePassword retrieves the safe storage password for Unix-like systems
func getSafeStoragePassword() (string, error) {
	// Allow tests/CI to override via environment variable
	if v := os.Getenv("COSTA_SAFE_STORAGE_PASSWORD"); v != "" {
		return v, nil
	}

	if runtime.GOOS == "darwin" {
		return getMacSafeStoragePassword()
	}
	if runtime.GOOS == "linux" {
		return getLinuxSafeStoragePassword()
	}
	return "", fmt.Errorf("unsupported platform for safe storage: %s", runtime.GOOS)
}

// getLinuxSafeStoragePassword retrieves the safe storage password on Linux
func getLinuxSafeStoragePassword() (string, error) {
	// Allow tests/CI to override via environment variable
	if v := os.Getenv("COSTA_SAFE_STORAGE_PASSWORD"); v != "" {
		return v, nil
	}

	services := []string{
		"Code Safe Storage",
		"Visual Studio Code Safe Storage",
		"VS Code Safe Storage",
		"Electron Safe Storage",
		"Chrome Safe Storage",
	}

	currentUser, _ := user.Current()
	accounts := []string{""}
	if currentUser != nil {
		accounts = append(accounts, currentUser.Username)
	}

	for _, service := range services {
		for _, account := range accounts {
			password, err := keyring.Get(service, account)
			if err == nil && strings.TrimSpace(password) != "" {
				return strings.TrimSpace(password), nil
			}
		}
	}

	return "", fmt.Errorf("could not find VS Code safe storage key in keyring (tried common service names)")
}

// getMacSafeStoragePassword retrieves the safe storage password on macOS
func getMacSafeStoragePassword() (string, error) {
	// Allow tests/CI to override via environment variable
	if v := os.Getenv("COSTA_SAFE_STORAGE_PASSWORD"); v != "" {
		return v, nil
	}

	services := []string{
		"Code Safe Storage",
		"Visual Studio Code Safe Storage",
		"VS Code Safe Storage",
		"Microsoft VS Code Safe Storage",
		"com.microsoft.VSCode Safe Storage",
		"Electron Safe Storage",
		"Chrome Safe Storage",
		"Chromium Safe Storage",
		"Code - OSS Safe Storage",
	}

	for _, service := range services {
		// #nosec G204 -- service is selected from a fixed allowlist above.
		out, err := exec.Command("security", "find-generic-password", "-s", service, "-w").Output()
		if err != nil {
			continue
		}
		password := strings.TrimSpace(string(out))
		if password != "" {
			return password, nil
		}
	}

	return "", fmt.Errorf("could not find VS Code safe storage key in Keychain (tried common service names)")
}

// pbkdf2SHA1 implements PBKDF2 with SHA1
func pbkdf2SHA1(password, salt []byte, iter, keyLen int) []byte {
	hashLen := sha1.Size
	numBlocks := (keyLen + hashLen - 1) / hashLen
	var out []byte
	for block := 1; block <= numBlocks; block++ {
		u := pbkdf2Block(password, salt, iter, block)
		out = append(out, u...)
	}
	return out[:keyLen]
}

// pbkdf2Block implements a single block of PBKDF2
func pbkdf2Block(password, salt []byte, iter, block int) []byte {
	var u []byte
	mac := hmac.New(sha1.New, password)
	mac.Write(salt)
	mac.Write(intToBigEndian(block))
	u = mac.Sum(nil)

	out := make([]byte, len(u))
	copy(out, u)

	for i := 1; i < iter; i++ {
		mac = hmac.New(sha1.New, password)
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range out {
			out[j] ^= u[j]
		}
	}

	return out
}

// intToBigEndian converts an integer to big-endian byte representation
func intToBigEndian(i int) []byte {
	return []byte{
		byte(i >> 24),
		byte(i >> 16),
		byte(i >> 8),
		byte(i),
	}
}

// pkcs7Pad implements PKCS#7 padding
func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - (len(data) % blockSize)
	if padLen == 0 {
		padLen = blockSize
	}
	padding := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, padding...)
}