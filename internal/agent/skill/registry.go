package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/tgifai/friday/internal/pkg/logs"
)

var (
	defaultSkills = []string{
		"chinese-copywriting", "github", "notion", "obsidian", "skill-creator", "summarize", "tmux",
	}
)

type Registry struct {
	agentDir   string
	builtinDir string
	skills     map[string]*Skill
	mu         sync.RWMutex

	autoLoad bool
	enabled  []string
	disabled []string
}

func NewRegistry(workspace string) *Registry {
	return &Registry{
		agentDir:   filepath.Join(workspace, "skills"),
		builtinDir: "skills",
		skills:     make(map[string]*Skill, 32),
	}
}

func (r *Registry) LoadAll() error {
	if err := r.LoadBuiltInSkills(); err != nil {
		return err
	}
	logs.Info("[skills] built-in skills loaded: %d", len(r.skills))

	if err := r.LoadAgentSkills(); err != nil {
		return err
	}
	logs.Info("[skills] total skills loaded: %d", len(r.skills))
	return nil
}

func (r *Registry) LoadBuiltInSkills() error {
	ctx := context.Background()
	if r.builtinDir == "" {
		logs.CtxWarn(ctx, "built-in skills directory not configured, skipping skill loading")
		return nil
	}
	return r.loadSkillsFromDir(ctx, r.builtinDir, "built-in")
}

func (r *Registry) LoadAgentSkills() error {
	ctx := context.Background()
	if r.agentDir == "" {
		return nil
	}
	return r.loadSkillsFromDir(ctx, r.agentDir, "agent")
}

func (r *Registry) loadSkillsFromDir(ctx context.Context, dir string, skillType string) error {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %s skills directory: %w", skillType, err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		logs.CtxDebug(ctx, "%s skills directory does not exist: %s", skillType, absPath)
		return nil
	}

	loadedCount := 0
	err = filepath.Walk(absPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || info.Name() != "SKILL.md" {
			return nil
		}

		oneSkill, parseErr := r.loadSkill(path)
		if parseErr != nil {
			logs.CtxWarn(ctx, "failed to load %s skill from %s: %v", skillType, path, parseErr)
			return nil
		}
		oneSkill.isBuiltIn = skillType == "built-in"

		if !r.shouldLoad(oneSkill.Name) {
			logs.CtxDebug(ctx, "%s skill %s is disabled, skipping", skillType, oneSkill.Name)
			return nil
		}

		r.mu.Lock()
		existing, exists := r.skills[oneSkill.Name]
		if exists {
			logs.CtxDebug(ctx, "overriding %s skill %s with %s skill", skillType, oneSkill.Name, skillType)
		}
		r.skills[oneSkill.Name] = oneSkill
		r.mu.Unlock()

		if exists {
			logs.CtxInfo(ctx, "loaded %s skill (override): %s (was: %s)", skillType, oneSkill.Name, existing.Path)
		} else {
			logs.CtxInfo(ctx, "loaded %s skill: %s", skillType, oneSkill.Name)
		}
		loadedCount++
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk %s skills directory: %w", skillType, err)
	}

	logs.CtxInfo(ctx, "loaded %d %s skills from %s", loadedCount, skillType, absPath)
	return nil
}

func (r *Registry) loadSkill(path string) (*Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill file: %w", err)
	}

	name, description, metadata, body, err := parseSkillFrontmatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse skill frontmatter: %w", err)
	}
	if name == "" {
		return nil, fmt.Errorf("skill name is required in frontmatter")
	}

	return &Skill{
		Name:        name,
		Description: description,
		Metadata:    metadata,
		Content:     body,
		Path:        path,
	}, nil
}

func parseSkillFrontmatter(content string) (string, string, map[string]interface{}, string, error) {
	startMarker := "---\n"
	endMarker := "\n---\n"

	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return "", "", nil, content, nil
	}

	endIdx := strings.Index(content[startIdx+len(startMarker):], endMarker)
	if endIdx == -1 {
		return "", "", nil, "", fmt.Errorf("frontmatter end marker not found")
	}
	endIdx += startIdx + len(startMarker)

	frontmatter := content[startIdx+len(startMarker) : endIdx]
	body := strings.TrimSpace(content[endIdx+len(endMarker):])

	var data struct {
		Name        string                 `yaml:"name"`
		Description string                 `yaml:"description"`
		Metadata    map[string]interface{} `yaml:"metadata"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &data); err != nil {
		return "", "", nil, "", fmt.Errorf("failed to parse frontmatter YAML: %w", err)
	}

	return data.Name, data.Description, data.Metadata, body, nil
}

func (r *Registry) shouldLoad(skillName string) bool {
	for _, disabled := range r.disabled {
		if disabled == skillName {
			return false
		}
	}
	if len(r.enabled) == 0 {
		return true
	}
	for _, enabled := range r.enabled {
		if enabled == skillName {
			return true
		}
	}
	return false
}

func (r *Registry) Get(name string) (*Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	oneSkill, exists := r.skills[name]
	if !exists {
		return nil, fmt.Errorf("skill not found: %s", name)
	}
	return oneSkill, nil
}

func (r *Registry) GetMultiple(names []string) ([]*Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0, len(names))
	missing := make([]string, 0)
	for _, name := range names {
		oneSkill, exists := r.skills[name]
		if !exists {
			missing = append(missing, name)
			continue
		}
		skills = append(skills, oneSkill)
	}

	if len(missing) > 0 {
		return skills, fmt.Errorf("skills not found: %v", missing)
	}
	return skills, nil
}

func (r *Registry) GetForAgent(ctx context.Context, skillNames []string) []*Skill {
	if len(skillNames) == 0 {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0, len(skillNames))
	for _, name := range skillNames {
		oneSkill, exists := r.skills[name]
		if !exists {
			logs.CtxWarn(ctx, "skill not found: %s", name)
			continue
		}
		skills = append(skills, oneSkill)
	}
	return skills
}

func (r *Registry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0, len(r.skills))
	for _, oneSkill := range r.skills {
		skills = append(skills, oneSkill)
	}
	return skills
}

func (r *Registry) BuildSkillsPrompt(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	parts := make([]string, 0, len(skills)*4+1)
	parts = append(parts, "# Available Skills\n")
	for i, oneSkill := range skills {
		parts = append(parts, fmt.Sprintf("## Skill: %s\n", oneSkill.Name))
		if oneSkill.Description != "" {
			parts = append(parts, fmt.Sprintf("**Description**: %s\n", oneSkill.Description))
		}
		if oneSkill.Content != "" {
			parts = append(parts, oneSkill.Content)
		}
		if i < len(skills)-1 {
			parts = append(parts, "\n\n---\n\n")
		}
	}

	return strings.Join(parts, "\n")
}

func (r *Registry) Reload(ctx context.Context) error {
	r.mu.Lock()
	r.skills = make(map[string]*Skill)
	r.mu.Unlock()
	return r.LoadAll()
}

func (r *Registry) ReloadAgentSkills(ctx context.Context) error {
	r.mu.Lock()
	agentSkillsToRemove := make([]string, 0)
	for name, oneSkill := range r.skills {
		if r.agentDir != "" && strings.HasPrefix(oneSkill.Path, r.agentDir) {
			agentSkillsToRemove = append(agentSkillsToRemove, name)
		}
	}
	for _, name := range agentSkillsToRemove {
		delete(r.skills, name)
	}
	r.mu.Unlock()

	return r.LoadAgentSkills()
}

func (r *Registry) GetBuiltInSkills() ([]*Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0, len(defaultSkills))
	for _, skillName := range defaultSkills {
		oneSkill, exists := r.skills[skillName]
		if exists {
			skills = append(skills, oneSkill)
		}
	}
	return skills, nil
}
