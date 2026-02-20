package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v3"

	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
)

var onboardHwd = &OnboardRunner{}

type OnboardRunner struct {
	scanner *bufio.Scanner
}

func (r *OnboardRunner) cmd() *cli.Command {
	return &cli.Command{
		Name:  "onboard",
		Usage: "Interactive setup wizard for first-time configuration",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "accept-risk",
				Usage: "Skip the disclaimer prompt",
			},
		},
		Action: r.run,
	}
}

// ── style helpers ──────────────────────────────────────────────────

var (
	cBanner  = color.New(color.FgCyan, color.Bold)
	cStep    = color.New(color.FgCyan, color.Bold)
	cWarn    = color.New(color.FgYellow)
	cSuccess = color.New(color.FgGreen)
	cError   = color.New(color.FgRed)
	cPrompt  = color.New(color.FgWhite, color.Bold)
	cDim     = color.New(color.FgHiBlack)
)

// ── provider metadata ──────────────────────────────────────────────

type providerMeta struct {
	Type       string
	DefaultURL string
	Model      string
}

var providerOptions = []providerMeta{
	{Type: "openai", DefaultURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"},
	{Type: "anthropic", DefaultURL: "https://api.anthropic.com", Model: "claude-sonnet-4-20250514"},
	{Type: "gemini", DefaultURL: "https://generativelanguage.googleapis.com/v1beta", Model: "gemini-2.5-flash"},
	{Type: "ollama", DefaultURL: "http://localhost:11434", Model: "llama3"},
	{Type: "qwen", DefaultURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-plus"},
}

// ── channel metadata ───────────────────────────────────────────────

type channelMeta struct {
	Type    string
	Prompts []struct {
		Key      string
		Label    string
		Required bool
	}
}

var channelOptions = []channelMeta{
	{
		Type: "telegram",
		Prompts: []struct {
			Key      string
			Label    string
			Required bool
		}{
			{Key: "token", Label: "Telegram Bot Token", Required: true},
		},
	},
	{
		Type: "lark",
		Prompts: []struct {
			Key      string
			Label    string
			Required bool
		}{
			{Key: "app_id", Label: "Lark App ID", Required: true},
			{Key: "app_secret", Label: "Lark App Secret", Required: true},
		},
	},
	{
		Type:    "http",
		Prompts: nil,
	},
}

// ── main flow ──────────────────────────────────────────────────────

func (r *OnboardRunner) run(ctx context.Context, cmd *cli.Command) error {
	r.scanner = bufio.NewScanner(os.Stdin)

	// check existing config
	cfgPath := consts.DefaultConfigPath()
	if _, err := os.Stat(cfgPath); err == nil {
		cWarn.Printf("  Config already exists at %s\n", cfgPath)
		if !r.confirm("  Overwrite existing config?", false) {
			fmt.Println("  Aborted.")
			return nil
		}
		fmt.Println()
	}

	// step 1: welcome + disclaimer
	if !cmd.Bool("accept-risk") {
		if !r.stepWelcome() {
			return nil
		}
	}

	// step 2: provider
	providerID, provCfg, pm, err := r.stepProvider()
	if err != nil {
		return err
	}

	// step 3: model
	primaryModel := r.stepModel(providerID, pm)

	// step 4: channel
	channelID, chCfg, err := r.stepChannel()
	if err != nil {
		return err
	}

	// step 5: auto-update
	autoUpdate := r.stepAutoUpdate()

	// step 6: confirm & write
	return r.stepConfirm(cfgPath, providerID, provCfg, pm, primaryModel, channelID, chCfg, autoUpdate)
}

// ── step 1: welcome ────────────────────────────────────────────────

func (r *OnboardRunner) stepWelcome() bool {
	fmt.Println()
	cBanner.Println("  ███████╗██████╗ ██╗██████╗  █████╗ ██╗   ██╗")
	cBanner.Println("  ██╔════╝██╔══██╗██║██╔══██╗██╔══██╗╚██╗ ██╔╝")
	cBanner.Println("  █████╗  ██████╔╝██║██║  ██║███████║ ╚████╔╝ ")
	cBanner.Println("  ██╔══╝  ██╔══██╗██║██║  ██║██╔══██║  ╚██╔╝  ")
	cBanner.Println("  ██║     ██║  ██║██║██████╔╝██║  ██║   ██║   ")
	cBanner.Println("  ╚═╝     ╚═╝  ╚═╝╚═╝╚═════╝ ╚═╝  ╚═╝   ╚═╝   ")
	cDim.Println("  Thank God It's Friday, Your Personal AI Assistant")
	fmt.Println()

	cWarn.Println("  ⚠  DISCLAIMER")
	fmt.Println()
	cWarn.Println("  Friday is an experimental AI agent project. By continuing,")
	cWarn.Println("  you acknowledge the following:")
	fmt.Println()
	cWarn.Println("  • Friday executes commands, reads/writes files, and sends")
	cWarn.Println("    messages on your behalf. Mistakes may occur.")
	cWarn.Println("  • You are responsible for reviewing actions that affect")
	cWarn.Println("    external systems (git push, API calls, messages).")
	cWarn.Println("  • API keys and tokens are stored locally in")
	cWarn.Printf("    %s. Keep this file secure.\n", consts.DefaultConfigPath())
	cWarn.Println("  • This software is provided \"as-is\" without warranty.")
	cWarn.Println("    Use at your own risk.")
	fmt.Println()

	if !r.confirm("  Do you accept these terms?", false) {
		fmt.Println()
		fmt.Println("  Aborted. You must accept the terms to continue.")
		return false
	}
	fmt.Println()
	return true
}

// ── step 2: provider ───────────────────────────────────────────────

func (r *OnboardRunner) stepProvider() (string, config.ProviderConfig, providerMeta, error) {
	r.printStepHeader("Step 2", "LLM Provider")

	cDim.Println("  Select provider type:")
	for i, p := range providerOptions {
		fmt.Printf("    [%d] %s\n", i+1, p.Type)
	}
	fmt.Println()

	idx := r.promptChoice("  Provider type", 1, len(providerOptions))
	pm := providerOptions[idx-1]
	fmt.Println()

	providerID := r.promptDefault("  Provider name", pm.Type+"-main")
	fmt.Println()

	apiKey := ""
	if pm.Type != "ollama" {
		apiKey = r.promptRequired("  API Key")
		fmt.Println()
	}

	baseURL := r.promptDefault("  Base URL", pm.DefaultURL)
	fmt.Println()

	provCfg := config.ProviderConfig{
		Type: pm.Type,
		Config: map[string]any{
			"api_key":       apiKey,
			"base_url":      baseURL,
			"default_model": pm.Model,
			"timeout":       60,
			"max_retries":   3,
		},
	}

	cSuccess.Printf("  ✓ Provider: %s (%s)\n\n", providerID, pm.Type)
	return providerID, provCfg, pm, nil
}

// ── step 3: model ──────────────────────────────────────────────────

func (r *OnboardRunner) stepModel(providerID string, pm providerMeta) string {
	r.printStepHeader("Step 3", "Model")

	model := r.promptDefault("  Model name", pm.Model)
	fmt.Println()

	fullSpec := providerID + ":" + model
	cSuccess.Printf("  ✓ Model: %s\n\n", fullSpec)
	return fullSpec
}

// ── step 4: channel ────────────────────────────────────────────────

func (r *OnboardRunner) stepChannel() (string, config.ChannelConfig, error) {
	r.printStepHeader("Step 4", "Channel")

	cDim.Println("  Select channel type:")
	for i, ch := range channelOptions {
		fmt.Printf("    [%d] %s\n", i+1, ch.Type)
	}
	fmt.Println()

	idx := r.promptChoice("  Channel type", 1, len(channelOptions))
	cm := channelOptions[idx-1]
	channelID := cm.Type + "-main"
	fmt.Println()

	chConfig := make(map[string]interface{})
	for _, p := range cm.Prompts {
		var val string
		if p.Required {
			val = r.promptRequired("  " + p.Label)
		} else {
			val = r.promptDefault("  "+p.Label, "")
		}
		chConfig[p.Key] = val
		fmt.Println()
	}

	chCfg := config.ChannelConfig{
		Type:    cm.Type,
		Enabled: true,
		Security: config.ChannelSecurityConfig{
			Policy:        "welcome",
			WelcomeWindow: 300,
			MaxResp:       3,
		},
		Config: chConfig,
	}

	cSuccess.Printf("  ✓ Channel: %s (%s)\n\n", channelID, cm.Type)
	return channelID, chCfg, nil
}

// ── step 5: auto-update ───────────────────────────────────────────

func (r *OnboardRunner) stepAutoUpdate() bool {
	r.printStepHeader("Step 5", "Auto Update")

	cDim.Println("  When enabled, Friday will periodically check for new")
	cDim.Println("  releases and update itself automatically.")
	fmt.Println()

	enabled := r.confirm("  Enable auto-update?", true)
	fmt.Println()

	if enabled {
		cSuccess.Println("  ✓ Auto-update: enabled")
	} else {
		cSuccess.Println("  ✓ Auto-update: disabled")
	}
	fmt.Println()
	return enabled
}

// ── step 6: confirm & write ────────────────────────────────────────

func (r *OnboardRunner) stepConfirm(
	cfgPath string,
	providerID string, provCfg config.ProviderConfig, pm providerMeta,
	primaryModel string,
	channelID string, chCfg config.ChannelConfig,
	autoUpdate bool,
) error {
	r.printStepHeader("Step 6", "Review")

	homeDir := consts.FridayHomeDir()
	workspaceDir := consts.DefaultWorkspaceDir()

	cDim.Printf("  Home directory:  %s\n", homeDir)
	cDim.Printf("  Config file:     %s\n", cfgPath)
	cDim.Printf("  Workspace:       %s\n", workspaceDir)
	fmt.Println()
	cDim.Printf("  Provider:     %s (%s)\n", providerID, pm.Type)
	cDim.Printf("  Model:        %s\n", primaryModel)
	cDim.Printf("  Channel:      %s (%s)\n", channelID, chCfg.Type)
	cDim.Printf("  Auto-update:  %v\n", autoUpdate)
	fmt.Println()

	if !r.confirm("  Write config and initialize workspace?", true) {
		fmt.Println("  Aborted.")
		return nil
	}
	fmt.Println()

	// build config
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Bind:                  "0.0.0.0:8088",
			MaxConcurrentSessions: 100,
			RequestTimeout:        300,
			AutoUpdate:            autoUpdate,
		},
		Logging: config.LoggingConfig{
			Level:      "info",
			Format:     "text",
			Output:     "both",
			File:       filepath.Join(consts.FridayHomeDir(), "logs", "friday.log"),
			MaxSize:    100,
			MaxBackups: 5,
			MaxAge:     3,
		},
		Agents: map[string]config.AgentConfig{
			"default": {
				Name:      "Default",
				Workspace: workspaceDir,
				Channels:  []string{channelID},
				Models: config.ModelsConfig{
					Primary: primaryModel,
				},
				Config: config.AgentRuntimeConfig{
					MaxIterations: 25,
					MaxTokens:     4000,
					Temperature:   0.7,
				},
			},
		},
		Channels:  map[string]config.ChannelConfig{channelID: chCfg},
		Providers: map[string]config.ProviderConfig{providerID: provCfg},
	}

	// write config
	if err := writeConfigDirect(cfgPath, cfg); err != nil {
		cError.Printf("  ✗ Failed to write config: %v\n", err)
		return err
	}
	cSuccess.Printf("  ✓ Created %s\n", cfgPath)

	// initialize workspace
	if err := initWorkspace(workspaceDir); err != nil {
		cError.Printf("  ✗ Failed to initialize workspace: %v\n", err)
		return err
	}
	cSuccess.Printf("  ✓ Initialized workspace at %s\n", workspaceDir)

	count := len(consts.WorkspaceMarkdownTemplates)
	cSuccess.Printf("  ✓ Created %d prompt template files\n", count)

	// clone global skills
	if err := initGlobalSkills(); err != nil {
		cWarn.Printf("  ⚠ Failed to clone skills repo: %v\n", err)
		cWarn.Println("    You can manually clone it later:")
		cWarn.Printf("    git clone %s %s\n", consts.SkillsRepoURL, consts.GlobalSkillsDir())
	} else {
		cSuccess.Printf("  ✓ Global skills initialized at %s\n", consts.GlobalSkillsDir())
	}

	fmt.Println()
	cSuccess.Println("  All set! Run \"friday gateway run\" to start.")
	fmt.Println()

	return nil
}

// ── workspace init ─────────────────────────────────────────────────

func initWorkspace(workspaceDir string) error {
	dirs := []string{
		workspaceDir,
		filepath.Join(workspaceDir, "memory"),
		filepath.Join(workspaceDir, "memory", "sessions"),
		filepath.Join(workspaceDir, "memory", "daily"),
		filepath.Join(workspaceDir, "skills"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	for name, tpl := range consts.WorkspaceMarkdownTemplates {
		path := filepath.Join(workspaceDir, name)
		if _, err := os.Stat(path); err == nil {
			continue // do not overwrite existing files
		}
		if err := os.WriteFile(path, []byte(tpl), 0o644); err != nil {
			return fmt.Errorf("write template %s: %w", name, err)
		}
	}

	return nil
}

func initGlobalSkills() error {
	dir := consts.GlobalSkillsDir()

	// already exists — try git pull instead
	if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
		cmd := exec.Command("git", "-C", dir, "pull", "--ff-only")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	cmd := exec.Command("git", "clone", "--depth", "1", consts.SkillsRepoURL, dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeConfigDirect(path string, cfg *config.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// Load into instance manager, apply, and save
	// Create a minimal valid config file first so Load succeeds
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		return err
	}
	if _, err := config.Load(path); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := config.Apply("config", cfg); err != nil {
		return fmt.Errorf("apply config: %w", err)
	}
	return config.Save()
}

// ── input helpers ──────────────────────────────────────────────────

func (r *OnboardRunner) prompt(label string) string {
	cPrompt.Printf("%s > ", label)
	if r.scanner.Scan() {
		return strings.TrimSpace(r.scanner.Text())
	}
	return ""
}

func (r *OnboardRunner) promptDefault(label string, defaultVal string) string {
	if defaultVal != "" {
		cPrompt.Printf("%s ", label)
		cDim.Printf("[%s]", defaultVal)
		cPrompt.Print(" > ")
	} else {
		cPrompt.Printf("%s > ", label)
	}

	if r.scanner.Scan() {
		val := strings.TrimSpace(r.scanner.Text())
		if val != "" {
			return val
		}
	}
	return defaultVal
}

func (r *OnboardRunner) promptRequired(label string) string {
	for {
		val := r.prompt(label)
		if val != "" {
			return val
		}
		cError.Println("  This field is required.")
	}
}

func (r *OnboardRunner) promptChoice(label string, min, max int) int {
	for {
		val := r.promptDefault(label, strconv.Itoa(min))
		n, err := strconv.Atoi(val)
		if err == nil && n >= min && n <= max {
			return n
		}
		cError.Printf("  Please enter a number between %d and %d.\n", min, max)
	}
}

func (r *OnboardRunner) confirm(label string, defaultYes bool) bool {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}

	cPrompt.Printf("%s %s > ", label, hint)
	if r.scanner.Scan() {
		val := strings.ToLower(strings.TrimSpace(r.scanner.Text()))
		if val == "" {
			return defaultYes
		}
		return val == "y" || val == "yes"
	}
	return defaultYes
}

func (r *OnboardRunner) printStepHeader(step string, title string) {
	cStep.Printf("═══ %s: %s ═══\n\n", step, title)
}
