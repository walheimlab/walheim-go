package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── expandHome ────────────────────────────────────────────────────────────────

func TestExpandHome_noTilde(t *testing.T) {
	got := expandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expandHome() = %q, want %q", got, "/absolute/path")
	}
}

func TestExpandHome_tilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandHome("~/foo/bar")

	want := filepath.Join(home, "foo/bar")
	if got != want {
		t.Errorf("expandHome(~/foo/bar) = %q, want %q", got, want)
	}
}

func TestExpandHome_tildeOnly(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandHome("~")
	// filepath.Join(home, "") == home
	if got != home {
		t.Errorf("expandHome(~) = %q, want %q", got, home)
	}
}

func TestExpandHome_empty(t *testing.T) {
	got := expandHome("")
	if got != "" {
		t.Errorf("expandHome('') = %q, want empty", got)
	}
}

// ── validate ──────────────────────────────────────────────────────────────────

func validConfig() *Config {
	return &Config{
		APIVersion: APIVersion,
		Kind:       Kind,
		Contexts:   []Context{{Name: "home", DataDir: "/data"}},
	}
}

func TestValidate_valid(t *testing.T) {
	if err := validConfig().validate(); err != nil {
		t.Errorf("validate() returned unexpected error: %v", err)
	}
}

func TestValidate_wrongAPIVersion(t *testing.T) {
	c := validConfig()
	c.APIVersion = "wrong/v1"

	err := c.validate()
	if err == nil {
		t.Fatal("expected error for wrong apiVersion")
	}

	if !strings.Contains(err.Error(), "apiVersion") {
		t.Errorf("error %q does not mention apiVersion", err.Error())
	}
}

func TestValidate_wrongKind(t *testing.T) {
	c := validConfig()
	c.Kind = "NotConfig"

	err := c.validate()
	if err == nil {
		t.Fatal("expected error for wrong kind")
	}

	if !strings.Contains(err.Error(), "kind") {
		t.Errorf("error %q does not mention kind", err.Error())
	}
}

func TestValidate_emptyContexts(t *testing.T) {
	c := validConfig()
	c.Contexts = nil

	err := c.validate()
	if err == nil {
		t.Fatal("expected error for empty contexts")
	}
}

func TestValidate_contextMissingName(t *testing.T) {
	c := validConfig()
	c.Contexts = []Context{{DataDir: "/data"}}

	err := c.validate()
	if err == nil {
		t.Fatal("expected error for context missing name")
	}
}

func TestValidate_contextMissingDataDir(t *testing.T) {
	c := validConfig()
	c.Contexts = []Context{{Name: "bad"}}

	err := c.validate()
	if err == nil {
		t.Fatal("expected error for context missing dataDir and s3")
	}
}

func TestValidate_duplicateContextName(t *testing.T) {
	c := validConfig()
	c.Contexts = []Context{
		{Name: "home", DataDir: "/data"},
		{Name: "home", DataDir: "/other"},
	}

	err := c.validate()
	if err == nil {
		t.Fatal("expected error for duplicate context name")
	}

	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error %q does not mention duplicate", err.Error())
	}
}

func TestValidate_currentContextNotFound(t *testing.T) {
	c := validConfig()
	c.CurrentContext = "nonexistent"

	err := c.validate()
	if err == nil {
		t.Fatal("expected error for currentContext not in contexts")
	}
}

func TestValidate_currentContextEmpty_valid(t *testing.T) {
	c := validConfig()

	c.CurrentContext = ""
	if err := c.validate(); err != nil {
		t.Errorf("empty currentContext should be valid: %v", err)
	}
}

// ── validateS3Config ──────────────────────────────────────────────────────────

func TestValidateS3Config_valid(t *testing.T) {
	s3 := &S3Config{Bucket: "my-bucket", Region: "eu-west-1"}
	if err := validateS3Config("ctx", s3); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateS3Config_missingBucket(t *testing.T) {
	s3 := &S3Config{Region: "us-east-1"}

	err := validateS3Config("ctx", s3)
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}

	if !strings.Contains(err.Error(), "bucket") {
		t.Errorf("error %q does not mention bucket", err.Error())
	}
}

func TestValidateS3Config_missingRegion(t *testing.T) {
	s3 := &S3Config{Bucket: "my-bucket"}

	err := validateS3Config("ctx", s3)
	if err == nil {
		t.Fatal("expected error for missing region")
	}

	if !strings.Contains(err.Error(), "region") {
		t.Errorf("error %q does not mention region", err.Error())
	}
}

// ── Context.IsS3 ──────────────────────────────────────────────────────────────

func TestContext_IsS3(t *testing.T) {
	local := Context{Name: "local", DataDir: "/data"}
	if local.IsS3() {
		t.Error("local context should not be S3")
	}

	s3 := Context{Name: "cloud", S3: &S3Config{}}
	if !s3.IsS3() {
		t.Error("S3 context should report IsS3()=true")
	}
}

// ── AddContext / DeleteContext / UseContext / DataDir / ContextForName ────────

func TestAddContext_duplicate(t *testing.T) {
	c := validConfig()

	err := c.AddContext("home", "/other", false)
	if err == nil {
		t.Fatal("expected error adding duplicate context name")
	}
}

func TestAddContext_activate(t *testing.T) {
	c := &Config{APIVersion: APIVersion, Kind: Kind}
	if err := c.AddContext("new", "/data", true); err != nil {
		t.Fatalf("AddContext: %v", err)
	}

	if c.CurrentContext != "new" {
		t.Errorf("CurrentContext = %q, want %q", c.CurrentContext, "new")
	}
}

func TestAddContext_noActivate(t *testing.T) {
	c := &Config{APIVersion: APIVersion, Kind: Kind, CurrentContext: "existing"}
	if err := c.AddContext("new", "/data", false); err != nil {
		t.Fatalf("AddContext: %v", err)
	}

	if c.CurrentContext != "existing" {
		t.Errorf("CurrentContext changed to %q unexpectedly", c.CurrentContext)
	}
}

func TestDeleteContext_notFound(t *testing.T) {
	c := validConfig()

	err := c.DeleteContext("nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent context")
	}
}

func TestDeleteContext_clearsCurrentContext(t *testing.T) {
	c := validConfig()

	c.CurrentContext = "home"
	if err := c.DeleteContext("home"); err != nil {
		t.Fatalf("DeleteContext: %v", err)
	}

	if c.CurrentContext != "" {
		t.Errorf("CurrentContext = %q after delete, want empty", c.CurrentContext)
	}

	if len(c.Contexts) != 0 {
		t.Errorf("expected 0 contexts, got %d", len(c.Contexts))
	}
}

func TestDeleteContext_doesNotClearOtherContext(t *testing.T) {
	c := &Config{
		APIVersion:     APIVersion,
		Kind:           Kind,
		CurrentContext: "other",
		Contexts: []Context{
			{Name: "home", DataDir: "/data"},
			{Name: "other", DataDir: "/other"},
		},
	}
	if err := c.DeleteContext("home"); err != nil {
		t.Fatalf("DeleteContext: %v", err)
	}

	if c.CurrentContext != "other" {
		t.Errorf("CurrentContext = %q, want %q", c.CurrentContext, "other")
	}
}

func TestUseContext_notFound(t *testing.T) {
	c := validConfig()
	if err := c.UseContext("missing"); err == nil {
		t.Fatal("expected error switching to nonexistent context")
	}
}

func TestUseContext_switches(t *testing.T) {
	c := &Config{
		APIVersion: APIVersion,
		Kind:       Kind,
		Contexts: []Context{
			{Name: "a", DataDir: "/a"},
			{Name: "b", DataDir: "/b"},
		},
	}
	if err := c.UseContext("b"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	if c.CurrentContext != "b" {
		t.Errorf("CurrentContext = %q, want %q", c.CurrentContext, "b")
	}
}

func TestDataDir_current(t *testing.T) {
	c := &Config{
		APIVersion:     APIVersion,
		Kind:           Kind,
		CurrentContext: "home",
		Contexts:       []Context{{Name: "home", DataDir: "/data/home"}},
	}

	got, err := c.DataDir("")
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}

	if got != "/data/home" {
		t.Errorf("DataDir() = %q, want %q", got, "/data/home")
	}
}

func TestDataDir_notFound(t *testing.T) {
	c := validConfig()

	_, err := c.DataDir("missing")
	if err == nil {
		t.Fatal("expected error for missing context")
	}
}

func TestContextForName_current(t *testing.T) {
	c := &Config{
		APIVersion:     APIVersion,
		Kind:           Kind,
		CurrentContext: "home",
		Contexts:       []Context{{Name: "home", DataDir: "/data"}},
	}

	ctx, err := c.ContextForName("")
	if err != nil {
		t.Fatalf("ContextForName: %v", err)
	}

	if ctx.Name != "home" {
		t.Errorf("Name = %q, want %q", ctx.Name, "home")
	}
}

// ── ListContexts ──────────────────────────────────────────────────────────────

func TestListContexts_localLocation(t *testing.T) {
	c := &Config{
		APIVersion:     APIVersion,
		Kind:           Kind,
		CurrentContext: "home",
		Contexts:       []Context{{Name: "home", DataDir: "/data/home"}},
	}

	views := c.ListContexts()
	if len(views) != 1 {
		t.Fatalf("expected 1 view, got %d", len(views))
	}

	v := views[0]
	if !v.Active {
		t.Error("expected Active=true for current context")
	}

	if v.Location != "/data/home" {
		t.Errorf("Location = %q, want %q", v.Location, "/data/home")
	}
}

func TestListContexts_s3Location(t *testing.T) {
	c := &Config{
		APIVersion: APIVersion,
		Kind:       Kind,
		Contexts: []Context{
			{Name: "cloud", S3: &S3Config{Bucket: "my-bucket", Prefix: "walheim"}},
		},
	}

	views := c.ListContexts()
	if views[0].Location != "s3://my-bucket/walheim" {
		t.Errorf("Location = %q, want %q", views[0].Location, "s3://my-bucket/walheim")
	}
}

func TestListContexts_s3NoPrefix(t *testing.T) {
	c := &Config{
		APIVersion: APIVersion,
		Kind:       Kind,
		Contexts: []Context{
			{Name: "cloud", S3: &S3Config{Bucket: "my-bucket"}},
		},
	}

	views := c.ListContexts()
	if views[0].Location != "s3://my-bucket" {
		t.Errorf("Location = %q, want %q", views[0].Location, "s3://my-bucket")
	}
}

// ── AddS3Context ──────────────────────────────────────────────────────────────

func TestAddS3Context_duplicate(t *testing.T) {
	c := validConfig()

	err := c.AddS3Context("home", S3Config{Bucket: "b", Region: "r"}, false)
	if err == nil {
		t.Fatal("expected error adding duplicate S3 context name")
	}
}

func TestAddS3Context_activate(t *testing.T) {
	c := &Config{APIVersion: APIVersion, Kind: Kind}
	if err := c.AddS3Context("cloud", S3Config{Bucket: "b", Region: "r"}, true); err != nil {
		t.Fatalf("AddS3Context: %v", err)
	}

	if c.CurrentContext != "cloud" {
		t.Errorf("CurrentContext = %q, want %q", c.CurrentContext, "cloud")
	}
}

// ── Load / Init / Save (file I/O with temp dir) ───────────────────────────────

func TestInit_and_Load(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")

	cfg, err := Init(cfgPath)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if cfg.APIVersion != APIVersion {
		t.Errorf("APIVersion = %q, want %q", cfg.APIVersion, APIVersion)
	}

	// Add a context so Load's validate() passes (needs at least one context).
	if err := cfg.AddContext("home", "/data", true); err != nil {
		t.Fatalf("AddContext: %v", err)
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.CurrentContext != "home" {
		t.Errorf("CurrentContext = %q, want %q", loaded.CurrentContext, "home")
	}
}

func TestLoad_missingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/to/config")
	if err == nil {
		t.Fatal("expected error loading nonexistent file")
	}
}

func TestSave_roundtrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")

	cfg, _ := Init(cfgPath)

	_ = cfg.AddContext("prod", "/data/prod", true)
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}

	if len(loaded.Contexts) != 1 || loaded.Contexts[0].Name != "prod" {
		t.Errorf("unexpected contexts after round-trip: %+v", loaded.Contexts)
	}
}
