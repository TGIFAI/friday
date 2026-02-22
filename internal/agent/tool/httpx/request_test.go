package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
)

func disableSSRF(t *testing.T) {
	t.Helper()
	orig := isPrivateHost
	isPrivateHost = func(string) bool { return false }
	t.Cleanup(func() { isPrivateHost = orig })
}

func TestRequestTool_Name(t *testing.T) {
	tool := NewRequestTool()
	if tool.Name() != "http_request" {
		t.Errorf("expected name http_request, got %s", tool.Name())
	}
}

func TestRequestTool_ToolInfo(t *testing.T) {
	tool := NewRequestTool()
	info := tool.ToolInfo()
	if info.Name != "http_request" {
		t.Errorf("expected ToolInfo name http_request, got %s", info.Name)
	}
}

func TestRequestTool_Execute_GET(t *testing.T) {
	disableSSRF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("X-Test", "hello")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tool := NewRequestTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    srv.URL,
		"method": "GET",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res requestResult
	if err := sonic.UnmarshalString(result.(string), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if res.Status != 200 {
		t.Errorf("expected status 200, got %d", res.Status)
	}
	if res.Body != `{"ok":true}` {
		t.Errorf("unexpected body: %s", res.Body)
	}
	if res.Headers["X-Test"] != "hello" {
		t.Errorf("expected header X-Test=hello, got %s", res.Headers["X-Test"])
	}
}

func TestRequestTool_Execute_POST(t *testing.T) {
	disableSSRF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	tool := NewRequestTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    srv.URL,
		"method": "POST",
		"body":   `{"name":"test"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res requestResult
	if err := sonic.UnmarshalString(result.(string), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if res.Status != 201 {
		t.Errorf("expected status 201, got %d", res.Status)
	}
}

func TestRequestTool_Execute_CustomHeaders(t *testing.T) {
	disableSSRF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token123" {
			t.Errorf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Errorf("expected Content-Type text/plain, got %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tool := NewRequestTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    srv.URL,
		"method": "POST",
		"body":   "hello",
		"headers": map[string]interface{}{
			"Authorization": "Bearer token123",
			"Content-Type":  "text/plain",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestTool_Execute_Truncation(t *testing.T) {
	disableSSRF(t)
	bigBody := strings.Repeat("x", maxResponseChar+100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(bigBody))
	}))
	defer srv.Close()

	tool := NewRequestTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    srv.URL,
		"method": "GET",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res requestResult
	if err := sonic.UnmarshalString(result.(string), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !res.Truncated {
		t.Error("expected truncated=true")
	}
	if res.Length != maxResponseChar {
		t.Errorf("expected length %d, got %d", maxResponseChar, res.Length)
	}
}

func TestRequestTool_Execute_InvalidURL(t *testing.T) {
	tool := NewRequestTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    "not-a-url",
		"method": "GET",
	})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestRequestTool_Execute_PrivateAddress(t *testing.T) {
	tool := NewRequestTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    "http://localhost:8080/secret",
		"method": "GET",
	})
	if err == nil {
		t.Fatal("expected error for private address")
	}
	if !strings.Contains(err.Error(), "private") {
		t.Errorf("expected private address error, got: %v", err)
	}
}

func TestRequestTool_Execute_UnsupportedMethod(t *testing.T) {
	tool := NewRequestTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    "https://example.com",
		"method": "TRACE",
	})
	if err == nil {
		t.Fatal("expected error for unsupported method")
	}
	if !strings.Contains(err.Error(), "unsupported method") {
		t.Errorf("expected unsupported method error, got: %v", err)
	}
}

func TestRequestTool_Execute_FTPScheme(t *testing.T) {
	tool := NewRequestTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    "ftp://example.com/file",
		"method": "GET",
	})
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
	if !strings.Contains(err.Error(), "only http and https") {
		t.Errorf("expected scheme error, got: %v", err)
	}
}

func TestRequestTool_Execute_MissingURL(t *testing.T) {
	tool := NewRequestTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"method": "GET",
	})
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestRequestTool_Execute_DELETE(t *testing.T) {
	disableSSRF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	tool := NewRequestTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    srv.URL,
		"method": "DELETE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res requestResult
	if err := sonic.UnmarshalString(result.(string), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if res.Status != 204 {
		t.Errorf("expected status 204, got %d", res.Status)
	}
}
