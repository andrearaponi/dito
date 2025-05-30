package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// predefinedHash is the expected SHA-256 hash of the public key for integrity validation.
const predefinedHash = "11ca5fb6bb6800ca1d286fc73985929836bc9412313d4bcd29dae219c9a81471"

// generateKeys creates a new Ed25519 key pair and saves them to disk.
func generateKeys() error {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate keys: %w", err)
	}

	if err := os.WriteFile("ed25519_public.key", publicKey, 0644); err != nil {
		return fmt.Errorf("failed to save public key: %w", err)
	}

	if err := os.WriteFile("ed25519_private.key", privateKey, 0600); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}

	fmt.Println("Keys generated successfully: ed25519_public.key, ed25519_private.key")
	return nil
}

// validatePublicKeyIntegrity checks that the public key file has not been altered.
func validatePublicKeyIntegrity() error {
	data, err := os.ReadFile("ed25519_public.key")
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != predefinedHash {
		return fmt.Errorf("public key integrity check failed")
	}

	return nil
}

// signFile generates a SHA-256 hash of the file and signs it with the private key.
func signFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	hash := sha256.Sum256(data)
	privateKey, err := os.ReadFile("ed25519_private.key")
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	signature := ed25519.Sign(privateKey, hash[:])
	if err := os.WriteFile(filename+".sig", []byte(hex.EncodeToString(signature)), 0644); err != nil {
		return fmt.Errorf("failed to save signature: %w", err)
	}

	fmt.Println("File signed successfully:", filename+".sig")
	return nil
}

// verifyFile checks the signature of the file using the public key.
func verifyFile(filename string) error {
	if err := validatePublicKeyIntegrity(); err != nil {
		return fmt.Errorf("public key integrity validation failed: %w", err)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	hash := sha256.Sum256(data)
	publicKey, err := os.ReadFile("ed25519_public.key")
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	sigData, err := os.ReadFile(filename + ".sig")
	if err != nil {
		return fmt.Errorf("failed to read signature file: %w", err)
	}

	signature, err := hex.DecodeString(string(sigData))
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	if !ed25519.Verify(publicKey, hash[:], signature) {
		return fmt.Errorf("signature verification failed")
	}

	fmt.Println("Signature verification successful")
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: plugin-signer [generate-keys | sign <file> | verify <file>]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate-keys":
		if err := generateKeys(); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	case "sign":
		if len(os.Args) < 3 {
			fmt.Println("Usage: plugin-signer sign <file>")
			os.Exit(1)
		}
		if err := signFile(os.Args[2]); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	case "verify":
		if len(os.Args) < 3 {
			fmt.Println("Usage: plugin-signer verify <file>")
			os.Exit(1)
		}
		if err := verifyFile(os.Args[2]); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	default:
		fmt.Println("Unknown command. Usage: plugin-signer [generate-keys | sign <file> | verify <file>]")
		os.Exit(1)
	}
}
