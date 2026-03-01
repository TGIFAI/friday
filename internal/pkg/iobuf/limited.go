package iobuf

// LimitedBuffer keeps only the first N bytes written, then silently discards
// the rest. It implements io.Writer.
type LimitedBuffer struct {
	max       int
	data      []byte
	truncated bool
}

func NewLimitedBuffer(maxBytes int) *LimitedBuffer {
	return &LimitedBuffer{max: maxBytes, data: make([]byte, 0, min(maxBytes, 64*1024))}
}

func (b *LimitedBuffer) Write(p []byte) (int, error) {
	if b.truncated {
		return len(p), nil
	}
	remaining := b.max - len(b.data)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.data = append(b.data, p[:remaining]...)
		b.truncated = true
	} else {
		b.data = append(b.data, p...)
	}
	return len(p), nil
}

func (b *LimitedBuffer) String() string { return string(b.data) }
func (b *LimitedBuffer) Bytes() []byte  { return b.data }
func (b *LimitedBuffer) Truncated() bool { return b.truncated }
