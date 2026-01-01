package auth

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"unicode/utf16"
)

// HashSHA1 hashes a string using SHA1 (legacy compatibility)
// This matches the behavior of HashHelper.GetHash(value, HashHelper.HashType.SHA1) in C#
// Note: C# uses UnicodeEncoding (UTF-16) for SHA1, not UTF-8
func HashSHA1(value string) string {
	// Convert string to UTF-16 (like C# UnicodeEncoding)
	runes := []rune(value)
	utf16Bytes := utf16.Encode(runes)
	
	// Convert []uint16 to []byte (little-endian, like C#)
	bytes := make([]byte, len(utf16Bytes)*2)
	for i, r := range utf16Bytes {
		bytes[i*2] = byte(r)
		bytes[i*2+1] = byte(r >> 8)
	}
	
	h := sha1.New()
	h.Write(bytes)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// HashMD5 hashes a string using MD5 (legacy compatibility)
// This matches the behavior of HashHelper.GetHash(value, HashHelper.HashType.MD5) in C#
func HashMD5(value string) string {
	// Note: MD5 is used for hashed_pkey in legacy system
	h := md5.New()
	h.Write([]byte(value))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// VerifyPassword verifies a password against a hash
func VerifyPassword(password, hash string) bool {
	hashedPassword := HashSHA1(password)
	return hashedPassword == hash
}

// HashPassword hashes a password (SHA1 for legacy compatibility)
func HashPassword(password string) string {
	return HashSHA1(password)
}

// HashResponse hashes a security question response (SHA1, lowercase)
func HashResponse(response string) string {
	// Legacy system converts to lowercase before hashing
	return HashSHA1(response)
}

