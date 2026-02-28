package browserx

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	defaultTimeout = 30 * time.Second
)

func (t *BrowserTool) opOpen(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	headless := true
	if v, ok := args["headless"]; ok {
		headless = gconv.To[bool](v)
	}

	sess, err := t.manager.openSession(headless)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"session_id": sess.ID,
		"headless":   sess.Headless,
		"stealth":    true,
	}, nil
}

func (t *BrowserTool) opClose(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sessionID := gconv.To[string](args["session_id"])
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	if err := t.manager.closeSession(sessionID); err != nil {
		return nil, err
	}

	return map[string]interface{}{"success": true}, nil
}

func (t *BrowserTool) opNavigate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	url := gconv.To[string](args["url"])
	if url == "" {
		return nil, fmt.Errorf("url is required for navigate")
	}

	waitLoad := true
	if v, ok := args["wait_load"]; ok {
		waitLoad = gconv.To[bool](v)
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if err := sess.Page.Navigate(url); err != nil {
		return nil, fmt.Errorf("navigate to %s: %w", url, err)
	}

	if waitLoad {
		if err := sess.Page.WaitLoad(); err != nil {
			logs.CtxWarn(ctx, "[tool:browser] WaitLoad warning: %v", err)
		}
	}

	info, err := sess.Page.Info()
	if err != nil {
		return nil, fmt.Errorf("get page info: %w", err)
	}

	return map[string]interface{}{
		"url":   info.URL,
		"title": info.Title,
	}, nil
}

func (t *BrowserTool) opClick(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	selector := gconv.To[string](args["selector"])
	if selector == "" {
		return nil, fmt.Errorf("selector is required for click")
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	el, err := t.findElement(sess.Page, selector, args)
	if err != nil {
		return nil, err
	}

	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, fmt.Errorf("click %s: %w", selector, err)
	}

	return map[string]interface{}{
		"success":  true,
		"selector": selector,
	}, nil
}

func (t *BrowserTool) opType(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	selector := gconv.To[string](args["selector"])
	if selector == "" {
		return nil, fmt.Errorf("selector is required for type")
	}

	text := gconv.To[string](args["text"])
	if text == "" {
		return nil, fmt.Errorf("text is required for type")
	}

	clearFirst := gconv.To[bool](args["clear"])

	sess.mu.Lock()
	defer sess.mu.Unlock()

	el, err := t.findElement(sess.Page, selector, args)
	if err != nil {
		return nil, err
	}

	if clearFirst {
		if err := el.SelectAllText(); err != nil {
			return nil, fmt.Errorf("select all text: %w", err)
		}
	}

	if err := el.Input(text); err != nil {
		return nil, fmt.Errorf("type into %s: %w", selector, err)
	}

	return map[string]interface{}{"success": true}, nil
}

func (t *BrowserTool) opScreenshot(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	format := proto.PageCaptureScreenshotFormatPng
	if f := gconv.To[string](args["format"]); strings.ToLower(f) == "jpeg" || strings.ToLower(f) == "jpg" {
		format = proto.PageCaptureScreenshotFormatJpeg
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	var data []byte

	selector := gconv.To[string](args["selector"])
	if selector != "" {
		el, findErr := t.findElement(sess.Page, selector, args)
		if findErr != nil {
			return nil, findErr
		}
		data, err = el.Screenshot(format, 90)
	} else {
		data, err = sess.Page.Screenshot(false, &proto.PageCaptureScreenshot{
			Format: format,
		})
	}

	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}

	return map[string]interface{}{
		"data":   base64.StdEncoding.EncodeToString(data),
		"format": string(format),
	}, nil
}

func (t *BrowserTool) opExtract(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	selector := gconv.To[string](args["selector"])
	if selector == "" {
		return nil, fmt.Errorf("selector is required for extract")
	}

	attribute := gconv.To[string](args["attribute"])
	all := gconv.To[bool](args["all"])

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if all {
		elements, findErr := sess.Page.Elements(selector)
		if findErr != nil {
			return nil, fmt.Errorf("find elements %s: %w", selector, findErr)
		}

		var results []map[string]interface{}
		for _, el := range elements {
			entry := map[string]interface{}{}
			if attribute != "" {
				val, _ := el.Attribute(attribute)
				if val != nil {
					entry["attribute"] = *val
				}
			}
			text, _ := el.Text()
			entry["text"] = text
			html, _ := el.HTML()
			entry["html"] = html
			results = append(results, entry)
		}

		return map[string]interface{}{
			"elements": results,
			"count":    len(results),
		}, nil
	}

	el, findErr := t.findElement(sess.Page, selector, args)
	if findErr != nil {
		return nil, findErr
	}

	result := map[string]interface{}{}
	if attribute != "" {
		val, _ := el.Attribute(attribute)
		if val != nil {
			result["attribute"] = *val
		}
	}
	text, _ := el.Text()
	result["text"] = text
	html, _ := el.HTML()
	result["html"] = html

	return result, nil
}

func (t *BrowserTool) opEvaluate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	script := gconv.To[string](args["script"])
	if script == "" {
		return nil, fmt.Errorf("script is required for evaluate")
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if !strings.HasPrefix(strings.TrimSpace(script), "(") && !strings.HasPrefix(strings.TrimSpace(script), "function") {
		script = fmt.Sprintf("() => { %s }", script)
	}

	result, err := sess.Page.Eval(script)
	if err != nil {
		return nil, fmt.Errorf("evaluate: %w", err)
	}

	return map[string]interface{}{
		"result": result.Value.Val(),
	}, nil
}

func (t *BrowserTool) opWait(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	selector := gconv.To[string](args["selector"])
	if selector == "" {
		return nil, fmt.Errorf("selector is required for wait")
	}

	timeout := defaultTimeout
	if v := gconv.To[float64](args["timeout"]); v > 0 {
		timeout = time.Duration(v * float64(time.Second))
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	page := sess.Page.Timeout(timeout)

	selectorType := gconv.To[string](args["selector_type"])
	var el *rod.Element
	if selectorType == "xpath" {
		el, err = page.ElementX(selector)
	} else {
		el, err = page.Element(selector)
	}

	if err != nil {
		return nil, fmt.Errorf("element not found: selector '%s' timed out after %v", selector, timeout)
	}

	if err := el.WaitVisible(); err != nil {
		return nil, fmt.Errorf("element '%s' not visible after %v", selector, timeout)
	}

	return map[string]interface{}{"success": true}, nil
}

func (t *BrowserTool) opScroll(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	selector := gconv.To[string](args["selector"])
	if selector != "" {
		el, findErr := t.findElement(sess.Page, selector, args)
		if findErr != nil {
			return nil, findErr
		}
		if err := el.ScrollIntoView(); err != nil {
			return nil, fmt.Errorf("scroll to element: %w", err)
		}
		return map[string]interface{}{"success": true}, nil
	}

	x := gconv.To[float64](args["x"])
	y := gconv.To[float64](args["y"])

	if err := sess.Page.Mouse.Scroll(x, y, 0); err != nil {
		return nil, fmt.Errorf("scroll: %w", err)
	}

	return map[string]interface{}{"success": true}, nil
}

func (t *BrowserTool) opListSessions(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sessions := t.manager.listSessions()
	return map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	}, nil
}

func (t *BrowserTool) requireSession(args map[string]interface{}) (*Session, error) {
	sessionID := gconv.To[string](args["session_id"])
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	return t.manager.getSession(sessionID)
}

func (t *BrowserTool) findElement(page *rod.Page, selector string, args map[string]interface{}) (*rod.Element, error) {
	selectorType := gconv.To[string](args["selector_type"])
	var el *rod.Element
	var err error

	timedPage := page.Timeout(defaultTimeout)

	if selectorType == "xpath" {
		el, err = timedPage.ElementX(selector)
	} else {
		el, err = timedPage.Element(selector)
	}

	if err != nil {
		return nil, fmt.Errorf("element not found: selector '%s' timed out after %v", selector, defaultTimeout)
	}
	return el, nil
}
