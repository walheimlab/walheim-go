package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	APIVersion        = "walheim.io/v1"
	Kind              = "Config"
	DefaultConfigPath = "~/.walheim/config"
	EnvVar            = "WHCONFIG"
)

// ConfigError is returned when config file reading/parsing fails.
type ConfigError struct {
	message string
}

func (e *ConfigError) Error() string {
	return e.message
}

// ValidationError is returned when config data is invalid.
type ValidationError struct {
	message string
}

func (e *ValidationError) Error() string {
	return e.message
}

// S3Config holds configuration for an S3-compatible storage backend.
// AccessKeyID and SecretAccessKey are optional; if omitted, the AWS SDK
// credential chain is used (AWS_ACCESS_KEY_ID env var, ~/.aws/credentials, etc.).
type S3Config struct {
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	Prefix          string `yaml:"prefix,omitempty"`
	AccessKeyID     string `yaml:"accessKeyID,omitempty"`
	SecretAccessKey string `yaml:"secretAccessKey,omitempty"`
}

// Context represents a single context in the config.
type Context struct {
	Name    string    `yaml:"name"`
	DataDir string    `yaml:"dataDir,omitempty"`
	S3      *S3Config `yaml:"s3,omitempty"`
}

// IsS3 reports whether this context uses an S3-compatible storage backend.
func (c *Context) IsS3() bool { return c.S3 != nil }

// ContextView is for listing contexts with "active" status.
type ContextView struct {
	Name     string
	DataDir  string
	S3       *S3Config
	Location string // display string: local path or "s3://bucket/prefix"
	Active   bool
}

// Config represents the entire ~/.walheim/config file.
type Config struct {
	APIVersion     string    `yaml:"apiVersion"`
	Kind           string    `yaml:"kind"`
	CurrentContext string    `yaml:"currentContext"`
	Contexts       []Context `yaml:"contexts"`
	path           string    // unexported: resolved config file path
}

// Init creates a fresh empty config file at the resolved path.
// The parent directory must exist. If the file already exists, it is overwritten.
// Returns the initialised (empty) Config ready for AddContext.
func Init(configPath string) (*Config, error) {
	var resolvedPath string
	if configPath != "" {
		resolvedPath = expandHome(configPath)
	} else if envPath := os.Getenv(EnvVar); envPath != "" {
		resolvedPath = expandHome(envPath)
	} else {
		resolvedPath = expandHome(DefaultConfigPath)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0755); err != nil {
		return nil, &ConfigError{
			message: fmt.Sprintf("failed to create config directory: %v", err),
		}
	}

	cfg := &Config{
		APIVersion: APIVersion,
		Kind:       Kind,
		Contexts:   []Context{},
		path:       resolvedPath,
	}

	// Write the skeleton file
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, &ConfigError{message: fmt.Sprintf("failed to marshal config: %v", err)}
	}
	if err := os.WriteFile(resolvedPath, data, 0600); err != nil {
		return nil, &ConfigError{
			message: fmt.Sprintf("failed to write config file %q: %v", resolvedPath, err),
		}
	}

	return cfg, nil
}

// Load reads config from the resolved path.
// Resolution order: explicit path arg > $WHCONFIG > ~/.walheim/config
func Load(configPath string) (*Config, error) {
	var resolvedPath string

	// Resolution order
	if configPath != "" {
		// Explicit path
		resolvedPath = expandHome(configPath)
	} else if envPath := os.Getenv(EnvVar); envPath != "" {
		// Environment variable
		resolvedPath = expandHome(envPath)
	} else {
		// Default
		resolvedPath = expandHome(DefaultConfigPath)
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, &ConfigError{
			message: fmt.Sprintf("failed to read config file %q: %v", resolvedPath, err),
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, &ConfigError{
			message: fmt.Sprintf("failed to parse config file: %v", err),
		}
	}

	cfg.path = resolvedPath

	// Validate
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Expand ~ in all context dataDirs
	for i := range cfg.Contexts {
		cfg.Contexts[i].DataDir = expandHome(cfg.Contexts[i].DataDir)
	}

	return &cfg, nil
}

// Save writes config atomically (write to temp file, then rename).
func (c *Config) Save() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return &ConfigError{
			message: fmt.Sprintf("failed to marshal config: %v", err),
		}
	}

	// Create temp file in same directory as target for atomic rename
	dir := filepath.Dir(c.path)
	tmpFile, err := os.CreateTemp(dir, ".walheim-config-*.tmp")
	if err != nil {
		return &ConfigError{
			message: fmt.Sprintf("failed to create temp file: %v", err),
		}
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return &ConfigError{
			message: fmt.Sprintf("failed to write temp file: %v", err),
		}
	}
	_ = tmpFile.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, c.path); err != nil {
		_ = os.Remove(tmpPath)
		return &ConfigError{
			message: fmt.Sprintf("failed to save config: %v", err),
		}
	}

	return nil
}

// DataDir returns the dataDir for the given context name.
// If contextName is empty, returns the current context's dataDir.
func (c *Config) DataDir(contextName string) (string, error) {
	if contextName == "" {
		contextName = c.CurrentContext
	}

	for _, ctx := range c.Contexts {
		if ctx.Name == contextName {
			return ctx.DataDir, nil
		}
	}

	return "", &ValidationError{
		message: fmt.Sprintf("context %q not found", contextName),
	}
}

// ContextForName returns the full Context for the given name.
// If name is empty, returns the current context.
func (c *Config) ContextForName(name string) (*Context, error) {
	if name == "" {
		name = c.CurrentContext
	}

	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			return &c.Contexts[i], nil
		}
	}

	return nil, &ValidationError{
		message: fmt.Sprintf("context %q not found", name),
	}
}

// AddS3Context adds a new S3-backed context.
// If activate is true, sets it as the current context.
func (c *Config) AddS3Context(name string, s3cfg S3Config, activate bool) error {
	for _, ctx := range c.Contexts {
		if ctx.Name == name {
			return &ValidationError{
				message: fmt.Sprintf("context %q already exists", name),
			}
		}
	}

	c.Contexts = append(c.Contexts, Context{
		Name: name,
		S3:   &s3cfg,
	})

	if activate {
		c.CurrentContext = name
	}

	return nil
}

// AddContext adds a new context.
// If activate is true, sets it as the current context.
func (c *Config) AddContext(name, dataDir string, activate bool) error {
	// Check for duplicates
	for _, ctx := range c.Contexts {
		if ctx.Name == name {
			return &ValidationError{
				message: fmt.Sprintf("context %q already exists", name),
			}
		}
	}

	c.Contexts = append(c.Contexts, Context{
		Name:    name,
		DataDir: expandHome(dataDir),
	})

	if activate {
		c.CurrentContext = name
	}

	return nil
}

// DeleteContext removes a context by name.
// If it's the current context, clears currentContext.
func (c *Config) DeleteContext(name string) error {
	idx := -1
	for i, ctx := range c.Contexts {
		if ctx.Name == name {
			idx = i
			break
		}
	}

	if idx == -1 {
		return &ValidationError{
			message: fmt.Sprintf("context %q not found", name),
		}
	}

	c.Contexts = append(c.Contexts[:idx], c.Contexts[idx+1:]...)

	if c.CurrentContext == name {
		c.CurrentContext = ""
	}

	return nil
}

// UseContext switches to a different active context.
func (c *Config) UseContext(name string) error {
	for _, ctx := range c.Contexts {
		if ctx.Name == name {
			c.CurrentContext = name
			return nil
		}
	}

	return &ValidationError{
		message: fmt.Sprintf("context %q not found", name),
	}
}

// ListContexts returns all contexts with "active" status.
func (c *Config) ListContexts() []ContextView {
	var result []ContextView
	for _, ctx := range c.Contexts {
		loc := ctx.DataDir
		if ctx.S3 != nil {
			loc = "s3://" + ctx.S3.Bucket
			if ctx.S3.Prefix != "" {
				loc += "/" + ctx.S3.Prefix
			}
		}
		result = append(result, ContextView{
			Name:     ctx.Name,
			DataDir:  ctx.DataDir,
			S3:       ctx.S3,
			Location: loc,
			Active:   ctx.Name == c.CurrentContext,
		})
	}
	return result
}

// validate checks config invariants.
func (c *Config) validate() error {
	if c.APIVersion != APIVersion {
		return &ValidationError{
			message: fmt.Sprintf("apiVersion must be %q, got %q", APIVersion, c.APIVersion),
		}
	}

	if c.Kind != Kind {
		return &ValidationError{
			message: fmt.Sprintf("kind must be %q, got %q", Kind, c.Kind),
		}
	}

	if len(c.Contexts) == 0 {
		return &ValidationError{
			message: "contexts list cannot be empty",
		}
	}

	// Check for duplicates and required fields
	seen := make(map[string]bool)
	for _, ctx := range c.Contexts {
		if ctx.Name == "" {
			return &ValidationError{
				message: "context name cannot be empty",
			}
		}
		if ctx.DataDir == "" && ctx.S3 == nil {
			return &ValidationError{
				message: fmt.Sprintf("context %q missing dataDir or s3 config", ctx.Name),
			}
		}
		if ctx.S3 != nil {
			if err := validateS3Config(ctx.Name, ctx.S3); err != nil {
				return err
			}
		}
		if seen[ctx.Name] {
			return &ValidationError{
				message: fmt.Sprintf("duplicate context name: %q", ctx.Name),
			}
		}
		seen[ctx.Name] = true
	}

	// If currentContext is set, it must exist
	if c.CurrentContext != "" && !seen[c.CurrentContext] {
		return &ValidationError{
			message: fmt.Sprintf("currentContext %q not found in contexts", c.CurrentContext),
		}
	}

	return nil
}

// validateS3Config checks required fields for an S3-backed context.
func validateS3Config(contextName string, s3 *S3Config) error {
	if s3.Bucket == "" {
		return &ValidationError{
			message: fmt.Sprintf("context %q: s3.bucket is required", contextName),
		}
	}
	if s3.Region == "" {
		return &ValidationError{
			message: fmt.Sprintf("context %q: s3.region is required", contextName),
		}
	}
	return nil
}

// expandHome expands ~ to the user's home directory.
func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
