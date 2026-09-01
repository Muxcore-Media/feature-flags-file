package internal

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	featureflagsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/featureflags/v1"
	"google.golang.org/grpc/metadata"
)

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version == "" {
		t.Error("module version must not be empty")
	}
	if info.HTTPAddr != ":9404" {
		t.Errorf("HTTPAddr = %q, want :9404", info.HTTPAddr)
	}
	if info.Description != "YAML file-backed feature flag provider with SIGHUP reload" {
		t.Errorf("unexpected description: %q", info.Description)
	}
}

func TestModuleLifecycle(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()

	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func writeFlags(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "flags.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestYAMLLoadIsEnabledVariantAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, `
beta:
  default: true
  variant: "b"
dark-mode:
  default: false
`)
	m := NewModule(Config{
		FilePath: path,
		GRPCAddr: "127.0.0.1:0",
		HTTPAddr: "127.0.0.1:0",
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	en, err := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "beta", DefaultValue: false})
	if err != nil {
		t.Fatal(err)
	}
	if !en.GetEnabled() {
		t.Fatal("beta should be enabled from YAML")
	}

	missing, err := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "unknown", DefaultValue: true})
	if err != nil {
		t.Fatal(err)
	}
	if !missing.GetEnabled() {
		t.Fatal("missing flag should use request default_value")
	}

	v, err := m.GetVariant(ctx, &featureflagsv1.GetVariantRequest{Flag: "beta", DefaultValue: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if v.GetVariant() != "b" {
		t.Fatalf("variant = %q want b", v.GetVariant())
	}

	vDef, err := m.GetVariant(ctx, &featureflagsv1.GetVariantRequest{Flag: "dark-mode", DefaultValue: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if vDef.GetVariant() != "off" {
		t.Fatalf("empty variant should fall back to default_value, got %q", vDef.GetVariant())
	}
}

func TestFEATURE_FLAGS_FILEEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, "env-flag:\n  default: true\n")
	t.Setenv("FEATURE_FLAGS_FILE", path)
	m := NewModule(Config{FilePath: "should-not-use.yaml", GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	if m.filePath != path {
		t.Fatalf("filePath = %q want %q", m.filePath, path)
	}
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	en, err := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "env-flag"})
	if err != nil || !en.GetEnabled() {
		t.Fatalf("env-flag: enabled=%v err=%v", en.GetEnabled(), err)
	}
}

func TestReloadUpdatesFlags(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, "flip:\n  default: false\n")
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	en, _ := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "flip"})
	if en.GetEnabled() {
		t.Fatal("expected false before reload")
	}
	if err := os.WriteFile(path, []byte("flip:\n  default: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.loadFile(); err != nil {
		t.Fatal(err)
	}
	en, _ = m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "flip"})
	if !en.GetEnabled() {
		t.Fatal("expected true after reload")
	}
}

func TestBadYAMLReloadKeepsOldFlags(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, "stable:\n  default: true\n")
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stable: [not-a-map\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.loadFile(); err == nil {
		t.Fatal("expected parse error")
	}
	en, err := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "stable"})
	if err != nil || !en.GetEnabled() {
		t.Fatalf("old flags should remain: enabled=%v err=%v", en.GetEnabled(), err)
	}
	if err := m.Health(ctx); err == nil {
		t.Fatal("Health should fail after bad reload")
	}
}

func TestMissingFileHealthFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.yaml")
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Health(ctx); err == nil {
		t.Fatal("Health should fail when flags file is missing")
	}
}

func TestHealthHTTPEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, "ok-flag:\n  default: true\n")
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	resp, err := http.Get("http://" + m.HTTPListenAddr() + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) == "" {
		t.Fatal("empty health body")
	}
}

func TestHealthHTTPDegradedOnLoadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.yaml")
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	resp, err := http.Get("http://" + m.HTTPListenAddr() + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d want 503", resp.StatusCode)
	}
}

func TestSettingsFlagsFileReload(t *testing.T) {
	dir := t.TempDir()
	pathA := writeFlags(t, dir, "a:\n  default: true\n")
	pathB := filepath.Join(dir, "flags-b.yaml")
	if err := os.WriteFile(pathB, []byte("b:\n  default: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewModule(Config{FilePath: pathA, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defs := m.Settings()
	if len(defs) != 2 || defs[0].Key != "flags_file" || defs[0].Value != pathA {
		t.Fatalf("Settings() = %+v", defs)
	}
	if err := m.UpdateSetting("flags_file", pathB); err != nil {
		t.Fatal(err)
	}
	en, err := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "b"})
	if err != nil || !en.GetEnabled() {
		t.Fatalf("flag b after path switch: enabled=%v err=%v", en.GetEnabled(), err)
	}
	enA, _ := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "a", DefaultValue: false})
	if enA.GetEnabled() {
		t.Fatal("flag a should be gone after switching files")
	}
}

func TestUpdateSettingRejectsBadPath(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, "keep:\n  default: true\n")
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope.yaml")
	if err := m.UpdateSetting("flags_file", missing); err == nil {
		t.Fatal("expected error for missing file")
	}
	for _, bad := range []string{"", ".", ".."} {
		if err := m.UpdateSetting("flags_file", bad); err == nil {
			t.Fatalf("expected error for path %q", bad)
		}
	}
	en, err := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "keep"})
	if err != nil || !en.GetEnabled() {
		t.Fatalf("flags should remain after failed update: enabled=%v err=%v", en.GetEnabled(), err)
	}
}

func TestUpdateSettingFlagsYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, "old:\n  default: false\n")
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	newYAML := "new-flag:\n  default: true\n  variant: v1\n"
	if err := m.UpdateSetting("flags_yaml", newYAML); err != nil {
		t.Fatal(err)
	}
	en, err := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "new-flag"})
	if err != nil || !en.GetEnabled() {
		t.Fatalf("new-flag enabled=%v err=%v", en.GetEnabled(), err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != strings.TrimSpace(newYAML) {
		t.Fatalf("file = %q want %q", string(data), newYAML)
	}
}

func TestUpdateSettingBadYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, "stable:\n  default: true\n")
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("flags_yaml", "broken: [\n"); err == nil {
		t.Fatal("expected parse error")
	}
	en, err := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "stable"})
	if err != nil || !en.GetEnabled() {
		t.Fatalf("flags should remain: enabled=%v err=%v", en.GetEnabled(), err)
	}
}

func TestEnabledForTargeting(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, `
targeted:
  default: false
  enabled_for:
    - admin-ui
`)
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	allowed := metadata.NewIncomingContext(ctx, metadata.Pairs("x-caller-id", "admin-ui"))
	denied := metadata.NewIncomingContext(ctx, metadata.Pairs("x-caller-id", "other-module"))

	en, err := m.IsEnabled(allowed, &featureflagsv1.IsEnabledRequest{Flag: "targeted"})
	if err != nil || !en.GetEnabled() {
		t.Fatalf("admin-ui should be enabled: enabled=%v err=%v", en.GetEnabled(), err)
	}
	en, err = m.IsEnabled(denied, &featureflagsv1.IsEnabledRequest{Flag: "targeted"})
	if err != nil || en.GetEnabled() {
		t.Fatalf("other-module should be disabled: enabled=%v err=%v", en.GetEnabled(), err)
	}
}

func TestPercentRolloutDeterministic(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, `
rollout:
  default: false
  percent: 50
`)
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	callerCtx := metadata.NewIncomingContext(ctx, metadata.Pairs("x-caller-id", "media-ui-app"))
	first, err := m.IsEnabled(callerCtx, &featureflagsv1.IsEnabledRequest{Flag: "rollout"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.IsEnabled(callerCtx, &featureflagsv1.IsEnabledRequest{Flag: "rollout"})
	if err != nil {
		t.Fatal(err)
	}
	if first.GetEnabled() != second.GetEnabled() {
		t.Fatal("percent rollout should be deterministic for the same caller")
	}
}

func TestSIGHUPReloadsFlags(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, "sighup:\n  default: false\n")
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	if err := os.WriteFile(path, []byte("sighup:\n  default: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("SIGHUP: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		en, err := m.IsEnabled(ctx, &featureflagsv1.IsEnabledRequest{Flag: "sighup"})
		if err == nil && en.GetEnabled() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("SIGHUP did not reload flags within timeout")
}
