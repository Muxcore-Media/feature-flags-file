package internal

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	featureflagsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/featureflags/v1"
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
	if info.HTTPAddr != ":9402" {
		t.Errorf("gRPC default addr = %q", info.HTTPAddr)
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
	if len(defs) != 1 || defs[0].Key != "flags_file" || defs[0].Value != pathA {
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
