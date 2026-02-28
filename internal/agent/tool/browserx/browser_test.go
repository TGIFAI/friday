package browserx

import (
	"context"
	"testing"

	"github.com/go-rod/rod/lib/launcher"
)

func TestBrowserToolIntegration(t *testing.T) {
	// Skip if no browser binary available.
	if _, found := launcher.LookPath(); !found {
		t.Skip("no browser binary found, skipping integration test")
	}

	tool := &BrowserTool{manager: newBrowserManager()}
	defer tool.manager.shutdown()
	ctx := context.Background()

	// 1. Open
	result, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "open",
		"headless":  true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	openResult := result.(map[string]interface{})
	sessionID := openResult["session_id"].(string)
	if sessionID == "" {
		t.Fatal("expected non-empty session_id")
	}
	if !openResult["stealth"].(bool) {
		t.Error("expected stealth=true")
	}

	// 2. List sessions
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation": "list_sessions",
	})
	if err != nil {
		t.Fatalf("list_sessions: %v", err)
	}
	listResult := result.(map[string]interface{})
	if listResult["count"].(int) != 1 {
		t.Errorf("expected 1 session, got %v", listResult["count"])
	}

	// 3. Navigate
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "navigate",
		"session_id": sessionID,
		"url":        "https://example.com",
	})
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	navResult := result.(map[string]interface{})
	if navResult["title"] == "" {
		t.Error("expected non-empty title after navigation")
	}

	// 4. Extract
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "extract",
		"session_id": sessionID,
		"selector":   "h1",
	})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	extractResult := result.(map[string]interface{})
	if extractResult["text"] == "" {
		t.Error("expected non-empty text from h1")
	}

	// 5. Screenshot
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "screenshot",
		"session_id": sessionID,
	})
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	ssResult := result.(map[string]interface{})
	if ssResult["data"] == "" {
		t.Error("expected non-empty screenshot data")
	}

	// 6. Evaluate
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "evaluate",
		"session_id": sessionID,
		"script":     "() => document.title",
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	evalResult := result.(map[string]interface{})
	if evalResult["result"] == nil {
		t.Error("expected non-nil evaluate result")
	}

	// 7. Close
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "close",
		"session_id": sessionID,
	})
	if err != nil {
		t.Fatalf("close: %v", err)
	}

	// 8. Verify session is gone
	_, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "navigate",
		"session_id": sessionID,
		"url":        "https://example.com",
	})
	if err == nil {
		t.Error("expected error when using closed session")
	}
}
