package utils

import (
	"crypto/rand"
	"encoding/binary"

	"github.com/bytedance/gopkg/lang/fastrand"
)

const randStrAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func RandDigits(n int) string {
	if n <= 0 {
		return ""
	}

	b := make([]byte, n)
	for i := range b {
		b[i] = byte('0' + fastrand.Uint32n(10))
	}

	return string(b)
}

// RandStr generates a cryptographically secure random alphanumeric string of length n.
func RandStr(n int) string {
	if n <= 0 {
		return ""
	}

	b := make([]byte, n)
	for i := range b {
		b[i] = randStrAlphabet[cryptoRandIntn(len(randStrAlphabet))]
	}
	return string(b)
}

func cryptoRandIntn(max int) int {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return int(binary.LittleEndian.Uint64(buf[:]) % uint64(max))
}

func Truncate(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

func Truncate80(content string) string {
	return Truncate(content, 80)
}
