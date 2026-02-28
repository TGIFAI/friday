package timex

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"
)

type TimeTool struct{}

func NewTimeTool() *TimeTool {
	return &TimeTool{}
}

func (t *TimeTool) Name() string {
	return "get_time"
}

func (t *TimeTool) Description() string {
	return "Get the current date and time. Use this tool to avoid hallucinating dates, times, or weekdays."
}

func (t *TimeTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"timezone": {
				Type: schema.String,
				Desc: "IANA timezone name (e.g. Asia/Shanghai, America/New_York, Europe/London). Defaults to the system local timezone if omitted.",
			},
		}),
	}
}

func (t *TimeTool) Execute(_ context.Context, args map[string]interface{}) (interface{}, error) {
	now := time.Now()

	if tz := strings.TrimSpace(gconv.To[string](args["timezone"])); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return nil, fmt.Errorf("invalid timezone %q: %w", tz, err)
		}
		now = now.In(loc)
	}

	zone, offset := now.Zone()
	hours := offset / 3600
	mins := (offset % 3600) / 60
	if mins < 0 {
		mins = -mins
	}

	return map[string]interface{}{
		"datetime":        now.Format(time.RFC3339),
		"date":            now.Format("2006-01-02"),
		"time":            now.Format("15:04:05"),
		"weekday":         now.Weekday().String(),
		"unix":            now.Unix(),
		"timezone":        zone,
		"timezone_offset": fmt.Sprintf("%+03d:%02d", hours, mins),
	}, nil
}
