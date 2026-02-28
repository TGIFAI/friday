package browserx

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"
)

// BrowserTool implements the tool.Tool interface for browser automation.
type BrowserTool struct {
	manager *BrowserManager
}

// NewBrowserTool creates a new BrowserTool backed by the global BrowserManager.
func NewBrowserTool() *BrowserTool {
	return &BrowserTool{
		manager: getGlobalManager(),
	}
}

func (t *BrowserTool) Name() string { return "browser" }

func (t *BrowserTool) Description() string {
	return "Browser automation tool with stealth anti-detection. Supports navigation, element interaction, screenshots, content extraction, and JavaScript evaluation."
}

func (t *BrowserTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"operation": {
				Type:     schema.String,
				Desc:     `Operation to perform: "open", "close", "navigate", "click", "type", "screenshot", "extract", "evaluate", "wait", "scroll", "list_sessions"`,
				Required: true,
			},
			"session_id": {
				Type: schema.String,
				Desc: "Browser session ID (returned by open, required for all operations except open and list_sessions)",
			},
			"url": {
				Type: schema.String,
				Desc: "URL to navigate to (for navigate operation)",
			},
			"selector": {
				Type: schema.String,
				Desc: "CSS or XPath selector (for click, type, extract, wait, scroll, screenshot)",
			},
			"selector_type": {
				Type: schema.String,
				Desc: `Selector type: "css" (default) or "xpath"`,
			},
			"text": {
				Type: schema.String,
				Desc: "Text to input (for type operation)",
			},
			"script": {
				Type: schema.String,
				Desc: "JavaScript code to execute (for evaluate operation)",
			},
			"headless": {
				Type: schema.Boolean,
				Desc: "Run browser in headless mode (default: true, for open operation)",
			},
			"format": {
				Type: schema.String,
				Desc: `Screenshot format: "png" (default) or "jpeg" (for screenshot operation)`,
			},
			"clear": {
				Type: schema.Boolean,
				Desc: "Clear existing text before typing (for type operation)",
			},
			"attribute": {
				Type: schema.String,
				Desc: "HTML attribute to extract (for extract operation, e.g. href, src)",
			},
			"all": {
				Type: schema.Boolean,
				Desc: "Extract all matching elements instead of first (for extract operation)",
			},
			"timeout": {
				Type: schema.Integer,
				Desc: "Timeout in seconds (for wait operation, default: 30)",
			},
			"x": {
				Type: schema.Number,
				Desc: "Horizontal scroll offset in pixels (for scroll operation)",
			},
			"y": {
				Type: schema.Number,
				Desc: "Vertical scroll offset in pixels (for scroll operation)",
			},
			"wait_load": {
				Type: schema.Boolean,
				Desc: "Wait for page load after navigation (default: true, for navigate operation)",
			},
		}),
	}
}

func (t *BrowserTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	operation := strings.ToLower(strings.TrimSpace(gconv.To[string](args["operation"])))

	switch operation {
	case "open":
		return t.opOpen(ctx, args)
	case "close":
		return t.opClose(ctx, args)
	case "navigate":
		return t.opNavigate(ctx, args)
	case "click":
		return t.opClick(ctx, args)
	case "type":
		return t.opType(ctx, args)
	case "screenshot":
		return t.opScreenshot(ctx, args)
	case "extract":
		return t.opExtract(ctx, args)
	case "evaluate":
		return t.opEvaluate(ctx, args)
	case "wait":
		return t.opWait(ctx, args)
	case "scroll":
		return t.opScroll(ctx, args)
	case "list_sessions":
		return t.opListSessions(ctx, args)
	default:
		return nil, fmt.Errorf("unknown operation %q, must be one of: open, close, navigate, click, type, screenshot, extract, evaluate, wait, scroll, list_sessions", operation)
	}
}

// Shutdown closes all browser sessions. Should be called on program exit.
func Shutdown() {
	getGlobalManager().shutdown()
}
