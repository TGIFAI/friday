package utils

import "github.com/bytedance/gopkg/lang/fastrand"

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

func Truncate(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

func Truncate80(content string) string {
	return Truncate(content, 80)
}
