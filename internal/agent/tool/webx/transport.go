package webx

import (
	"io"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
	kflate "github.com/klauspost/compress/flate"
	kgzip "github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

// compressedTransport wraps an http.RoundTripper to advertise
// gzip, deflate, br, zstd via Accept-Encoding and transparently
// decompress the response body.
type compressedTransport struct {
	base http.RoundTripper
}

// newCompressedTransport creates a compression-aware transport.
// If base is nil, a default http.Transport (with DisableCompression) is used.
// When an *http.Transport is provided, DisableCompression is forced to true
// so the standard library does not interfere with our own decompression.
func newCompressedTransport(base *http.Transport) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport.(*http.Transport).Clone()
	}
	base.DisableCompression = true
	return &compressedTransport{base: base}
}

const acceptEncoding = "gzip, deflate, br, zstd"

func (t *compressedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only inject Accept-Encoding when the caller hasn't set it explicitly.
	if req.Header.Get("Accept-Encoding") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	ce := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if ce == "" {
		return resp, nil
	}

	var reader io.ReadCloser
	switch ce {
	case "gzip":
		r, err := kgzip.NewReader(resp.Body)
		if err != nil {
			return resp, nil // fallback: return raw body
		}
		reader = &decompressReader{reader: r, closer: resp.Body}
	case "deflate":
		r := kflate.NewReader(resp.Body)
		reader = &decompressReader{reader: r, closer: resp.Body}
	case "br":
		reader = &decompressReader{reader: brotli.NewReader(resp.Body), closer: resp.Body}
	case "zstd":
		r, err := zstd.NewReader(resp.Body)
		if err != nil {
			return resp, nil
		}
		reader = &zstdReadCloser{decoder: r, body: resp.Body}
	default:
		return resp, nil
	}

	resp.Body = reader
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length") // length no longer valid after decompression
	resp.ContentLength = -1
	return resp, nil
}

// decompressReader wraps a decompression io.Reader and the original resp.Body
// so that Close() properly releases both.
type decompressReader struct {
	reader io.Reader
	closer io.Closer // original resp.Body
}

func (d *decompressReader) Read(p []byte) (int, error) {
	return d.reader.Read(p)
}

func (d *decompressReader) Close() error {
	if c, ok := d.reader.(io.Closer); ok {
		_ = c.Close()
	}
	return d.closer.Close()
}

// zstdReadCloser handles zstd.Decoder which has its own Close semantics.
type zstdReadCloser struct {
	decoder *zstd.Decoder
	body    io.Closer
}

func (z *zstdReadCloser) Read(p []byte) (int, error) {
	return z.decoder.Read(p)
}

func (z *zstdReadCloser) Close() error {
	z.decoder.Close()
	return z.body.Close()
}
