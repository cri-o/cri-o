package sprig

import (
	"crypto/sha256"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/adler32"
)

func sha256sum(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

func sha1sum(input string) string {
	hash := sha256Sum([]byte(input))
	return hex.EncodeToString(hash[:])
}

func adler32sum(input string) string {
	hash := adler32.Checksum([]byte(input))
	return fmt.Sprintf("%d", hash)
}
