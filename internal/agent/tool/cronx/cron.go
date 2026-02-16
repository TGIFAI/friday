package cronx

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/cronjob"
	"github.com/tgifai/friday/internal/pkg/logs"
)

type CronTool struct{}

func NewCronTool() *CronTool {
	return &CronTool{}
}

func (t *CronTool) Name() string {
	return "cronx"
}

func (t *CronTool) Description() string {
	return "Manage scheduled cron jobs: create, list, delete, or update periodic and one-shot tasks"
}

func (t *CronTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     `Action to perform: "create", "list", "delete", or "update"`,
				Required: true,
			},
			"job_id": {
				Type: schema.String,
				Desc: `Job ID (required for delete/update)`,
			},
			"name": {
				Type: schema.String,
				Desc: `Human-readable job name (required for create)`,
			},
			"schedule_type": {
				Type: schema.String,
				Desc: `Schedule type: "every" (fixed interval like "5m", "1h30m"), "cron" (5-field cron expression like "0 9 * * *"), or "at" (one-shot ISO 8601 timestamp like "2026-03-01T09:00:00Z"). Required for create.`,
			},
			"schedule": {
				Type: schema.String,
				Desc: `Schedule value matching the schedule_type. Required for create.`,
			},
			"prompt": {
				Type: schema.String,
				Desc: `The message/instruction sent to the agent when the job fires. Required for create.`,
			},
			"session_target": {
				Type: schema.String,
				Desc: `"main" to share the agent's primary conversation, or "isolated" for a dedicated session (default: "isolated")`,
			},
			"channel_id": {
				Type: schema.String,
				Desc: `Delivery channel ID for isolated jobs (where to send the result). Defaults to the current channel.`,
			},
			"chat_id": {
				Type: schema.String,
				Desc: `Delivery chat ID for isolated jobs. Defaults to the current chat.`,
			},
			"enabled": {
				Type: schema.Boolean,
				Desc: `Enable or disable the job (used with update, default: true for create)`,
			},
		}),
	}
}

func (t *CronTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	scheduler := cronjob.Default()
	if scheduler == nil {
		return nil, fmt.Errorf("cron scheduler is not initialized")
	}

	action := strings.ToLower(strings.TrimSpace(gconv.To[string](args["action"])))
	switch action {
	case "create":
		return t.create(ctx, scheduler, args)
	case "list":
		return t.list(ctx, scheduler)
	case "delete":
		return t.delete(ctx, scheduler, args)
	case "update":
		return t.update(ctx, scheduler, args)
	default:
		return nil, fmt.Errorf("unknown action %q, must be one of: create, list, delete, update", action)
	}
}

func (t *CronTool) create(ctx context.Context, s *cronjob.Scheduler, args map[string]interface{}) (interface{}, error) {
	name := gconv.To[string](args["name"])
	if name == "" {
		return nil, fmt.Errorf("name is required for create")
	}
	schedType := gconv.To[string](args["schedule_type"])
	if schedType == "" {
		return nil, fmt.Errorf("schedule_type is required for create")
	}
	schedule := gconv.To[string](args["schedule"])
	if schedule == "" {
		return nil, fmt.Errorf("schedule is required for create")
	}
	prompt := gconv.To[string](args["prompt"])
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required for create")
	}

	sessionTarget := cronjob.SessionIsolated
	if st := gconv.To[string](args["session_target"]); st == "main" {
		sessionTarget = cronjob.SessionMain
	}

	// Resolve agent_id, channel_id, chat_id from context.
	agentID, _ := ctx.Value(consts.CtxKeyAgentID).(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id not found in context")
	}

	channelID := gconv.To[string](args["channel_id"])
	if channelID == "" {
		channelID, _ = ctx.Value(consts.CtxKeyChannelID).(string)
	}
	chatID := gconv.To[string](args["chat_id"])
	if chatID == "" {
		chatID, _ = ctx.Value(consts.CtxKeyChatID).(string)
	}

	jobID := fmt.Sprintf("cronx-%s-%d", sanitizeID(name), time.Now().UnixMilli())

	job := cronjob.Job{
		ID:            jobID,
		Name:          name,
		AgentID:       agentID,
		ScheduleType:  cronjob.ScheduleType(schedType),
		Schedule:      schedule,
		Prompt:        prompt,
		SessionTarget: sessionTarget,
		ChannelID:     channelID,
		ChatID:        chatID,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}

	if err := s.AddJob(job, true); err != nil {
		return nil, fmt.Errorf("add job: %w", err)
	}

	logs.CtxInfo(ctx, "[tool:cronx] created job %s (%s) schedule=%s:%s", jobID, name, schedType, schedule)

	return map[string]interface{}{
		"success": true,
		"job_id":  jobID,
		"name":    name,
		"message": fmt.Sprintf("Job %q created successfully", name),
	}, nil
}

func (t *CronTool) list(_ context.Context, s *cronjob.Scheduler) (interface{}, error) {
	jobs := s.ListJobs()
	result := make([]map[string]interface{}, 0, len(jobs))
	for _, j := range jobs {
		entry := map[string]interface{}{
			"job_id":         j.ID,
			"name":           j.Name,
			"agent_id":       j.AgentID,
			"schedule_type":  string(j.ScheduleType),
			"schedule":       j.Schedule,
			"session_target": string(j.SessionTarget),
			"enabled":        j.Enabled,
			"created_at":     j.CreatedAt.Format(time.RFC3339),
		}
		if j.LastRunAt != nil {
			entry["last_run_at"] = j.LastRunAt.Format(time.RFC3339)
		}
		if j.NextRunAt != nil {
			entry["next_run_at"] = j.NextRunAt.Format(time.RFC3339)
		}
		if j.Prompt != "" {
			// Truncate long prompts in listing.
			p := j.Prompt
			if len(p) > 120 {
				p = p[:120] + "..."
			}
			entry["prompt"] = p
		}
		result = append(result, entry)
	}
	return map[string]interface{}{
		"jobs":  result,
		"count": len(result),
	}, nil
}

func (t *CronTool) delete(ctx context.Context, s *cronjob.Scheduler, args map[string]interface{}) (interface{}, error) {
	jobID := gconv.To[string](args["job_id"])
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required for delete")
	}

	if cronjob.IsHeartbeatJob(jobID) {
		return nil, fmt.Errorf("cannot delete built-in heartbeat job")
	}

	if err := s.RemoveJob(jobID); err != nil {
		return nil, fmt.Errorf("remove job: %w", err)
	}

	logs.CtxInfo(ctx, "[tool:cronx] deleted job %s", jobID)
	return map[string]interface{}{
		"success": true,
		"job_id":  jobID,
		"message": fmt.Sprintf("Job %q deleted", jobID),
	}, nil
}

func (t *CronTool) update(ctx context.Context, s *cronjob.Scheduler, args map[string]interface{}) (interface{}, error) {
	jobID := gconv.To[string](args["job_id"])
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required for update")
	}

	if cronjob.IsHeartbeatJob(jobID) {
		return nil, fmt.Errorf("cannot update built-in heartbeat job")
	}

	jobs := s.ListJobs()
	var found *cronjob.Job
	for i := range jobs {
		if jobs[i].ID == jobID {
			found = &jobs[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("job %q not found", jobID)
	}

	updated := false

	if v, ok := args["name"]; ok {
		found.Name = gconv.To[string](v)
		updated = true
	}
	if v, ok := args["schedule_type"]; ok {
		found.ScheduleType = cronjob.ScheduleType(gconv.To[string](v))
		updated = true
	}
	if v, ok := args["schedule"]; ok {
		found.Schedule = gconv.To[string](v)
		updated = true
	}
	if v, ok := args["prompt"]; ok {
		found.Prompt = gconv.To[string](v)
		updated = true
	}
	if v, ok := args["session_target"]; ok {
		found.SessionTarget = cronjob.SessionTarget(gconv.To[string](v))
		updated = true
	}
	if v, ok := args["channel_id"]; ok {
		found.ChannelID = gconv.To[string](v)
		updated = true
	}
	if v, ok := args["chat_id"]; ok {
		found.ChatID = gconv.To[string](v)
		updated = true
	}
	if v, ok := args["enabled"]; ok {
		found.Enabled = gconv.To[bool](v)
		updated = true
	}

	if !updated {
		return nil, fmt.Errorf("no fields to update")
	}

	// Re-add with updated fields (AddJob deduplicates, so use the update path).
	if err := s.UpdateJob(*found); err != nil {
		return nil, fmt.Errorf("update job: %w", err)
	}

	logs.CtxInfo(ctx, "[tool:cronx] updated job %s", jobID)
	return map[string]interface{}{
		"success": true,
		"job_id":  jobID,
		"message": fmt.Sprintf("Job %q updated", jobID),
	}, nil
}

// sanitizeID produces a slug-friendly string from a name.
func sanitizeID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, name)
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}
