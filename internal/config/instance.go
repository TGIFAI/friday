package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	lockRetryInterval  = 50 * time.Millisecond
	lockAcquireTimeout = 5 * time.Second
	lockStaleAfter     = 30 * time.Second
	maxBackupFiles     = 5

	defaultConfigPath = "config.yaml"
)

var defaultManager = &InstanceManager{}

var ErrConfigConflict = errors.New("config conflict")

type InstanceManager struct {
	path string
	// loaded indicates whether Load has been called successfully.
	loaded bool
	cfg    *Config
	// hash tracks the current in-memory config snapshot hash.
	hash string

	mu sync.RWMutex
}

func (ins *InstanceManager) Get() (*Config, error) {
	if ins == nil {
		return nil, fmt.Errorf("instance manager is nil")
	}

	ins.mu.RLock()
	defer ins.mu.RUnlock()

	if !ins.loaded || ins.cfg == nil {
		return nil, fmt.Errorf("config is not loaded")
	}

	return ins.cfg.Clone()
}

func (ins *InstanceManager) Load(path string) (*Config, error) {
	if ins == nil {
		return nil, fmt.Errorf("instance manager is nil")
	}

	ins.mu.Lock()
	defer ins.mu.Unlock()

	path = strings.TrimSpace(path)
	if path == "" {
		if strings.TrimSpace(ins.path) != "" {
			path = ins.path
		} else {
			path = defaultConfigPath
		}
	}

	cfg, err := loadConfigFile(path)
	if err != nil {
		return nil, err
	}

	ins.path = path
	ins.cfg = cfg
	cfgHash := cfg.Hash()
	ins.hash = cfgHash
	ins.loaded = true
	return cfg.Clone()
}

func (ins *InstanceManager) Apply(name string, value any) error {
	return ins.ApplyWithCAS(name, value, "")
}

func (ins *InstanceManager) ApplyWithCAS(name string, value any, expectedHash string) error {
	if ins == nil {
		return fmt.Errorf("instance manager is nil")
	}

	ins.mu.Lock()
	defer ins.mu.Unlock()

	if !ins.loaded || ins.cfg == nil {
		return fmt.Errorf("config is not loaded")
	}

	expectedHash = strings.TrimSpace(expectedHash)
	if expectedHash != "" && expectedHash != ins.hash {
		return fmt.Errorf("%w: expected %s, got %s", ErrConfigConflict, expectedHash, ins.hash)
	}

	draft, err := ins.cfg.Clone()
	if err != nil {
		return err
	}

	if err := draft.UpdateByName(name, value); err != nil {
		return err
	}

	if err := draft.Validate(); err != nil {
		return err
	}

	ins.cfg = draft
	ins.hash = draft.Hash()
	return nil
}

func (ins *InstanceManager) Hash() (string, error) {
	if ins == nil {
		return "", fmt.Errorf("instance manager is nil")
	}

	ins.mu.RLock()
	defer ins.mu.RUnlock()

	if !ins.loaded || ins.cfg == nil {
		return "", fmt.Errorf("config is not loaded")
	}

	return ins.hash, nil
}

func (ins *InstanceManager) Save() error {
	if ins == nil {
		return fmt.Errorf("instance manager is nil")
	}

	ins.mu.Lock()
	defer ins.mu.Unlock()

	if !ins.loaded || ins.cfg == nil {
		return fmt.Errorf("config is not loaded")
	}

	savedHash, err := ins.saveConfig(ins.cfg)
	if err != nil {
		return err
	}
	ins.hash = savedHash
	return nil
}

func (ins *InstanceManager) saveConfig(cfg *Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config cannot be nil")
	}

	path := strings.TrimSpace(ins.path)
	if path == "" {
		return "", fmt.Errorf("config path is required")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}

	unlock, err := acquireFileLock(path+".lock", lockAcquireTimeout, lockStaleAfter)
	if err != nil {
		return "", fmt.Errorf("acquire config file lock: %w", err)
	}
	defer unlock()

	newHash := cfg.Hash()
	raw, err := marshalConfigYAML(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}

	mode := os.FileMode(0o644)
	hasPrev := false
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
		hasPrev = true
	} else if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("stat config file: %w", statErr)
	}

	if hasPrev {
		_, err := createBackup(path, mode)
		if err != nil {
			return "", err
		}
		go cleanupOldBackups(path)
	}

	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return "", fmt.Errorf("create temp config file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(raw); err != nil {
		_ = tmpFile.Close()
		return "", fmt.Errorf("write temp config file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp config file: %w", err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return "", fmt.Errorf("chmod temp config file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("replace config file: %w", err)
	}

	cleanup = false
	return newHash, nil
}

func loadConfigFile(path string) (*Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("config path is required")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func Load(path string) (*Config, error) {
	return defaultManager.Load(path)
}

func Get() (*Config, error) {
	return defaultManager.Get()
}

func Apply(name string, value any) error {
	return defaultManager.Apply(name, value)
}

func ApplyWithCAS(name string, value any, expectedHash string) error {
	return defaultManager.ApplyWithCAS(name, value, expectedHash)
}

func Save() error {
	return defaultManager.Save()
}

func Hash() (string, error) {
	return defaultManager.Hash()
}

func acquireFileLock(lockPath string, timeout, staleAfter time.Duration) (func(), error) {
	start := time.Now()
	for {
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = lockFile.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
			_ = lockFile.Close()
			return func() {
				_ = os.Remove(lockPath)
			}, nil
		}

		if !os.IsExist(err) {
			return nil, err
		}

		if staleAfter > 0 {
			if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > staleAfter {
				_ = os.Remove(lockPath)
				continue
			}
		}

		if timeout > 0 && time.Since(start) > timeout {
			return nil, fmt.Errorf("lock timeout after %s", timeout)
		}

		time.Sleep(lockRetryInterval)
	}
}

func createBackup(path string, mode os.FileMode) (string, error) {
	backupPath, err := nextBackupPath(path)
	if err != nil {
		return "", err
	}

	src, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open source config for backup: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return "", fmt.Errorf("create config backup file: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("copy config backup: %w", err)
	}

	if err := dst.Close(); err != nil {
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("close config backup file: %w", err)
	}

	return backupPath, nil
}

func nextBackupPath(path string) (string, error) {
	stamp := time.Now().Format("060102150405")
	candidate := fmt.Sprintf("%s.%s", path, stamp)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, nil
	} else if err != nil {
		return "", fmt.Errorf("stat backup path: %w", err)
	}

	for i := 1; ; i++ {
		one := fmt.Sprintf("%s.%d", candidate, i)
		if _, err := os.Stat(one); os.IsNotExist(err) {
			return one, nil
		} else if err != nil {
			return "", fmt.Errorf("stat backup path: %w", err)
		}
	}
}

func cleanupOldBackups(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}

	pattern := fmt.Sprintf("%s.*", path)
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) <= maxBackupFiles {
		return
	}

	sort.Strings(files)

	toDelete := len(files) - maxBackupFiles
	for i := 0; i < toDelete; i++ {
		_ = os.Remove(files[i])
	}
}

func marshalConfigYAML(cfg *Config) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(cfg); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}

	content := strings.TrimRight(buf.String(), "\n")
	return []byte(content + "\n"), nil
}
