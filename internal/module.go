package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	"github.com/Muxcore-Media/core/pkg/contracts"
	featureflagsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/featureflags/v1"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
)

// Version is set from cmd/module via -ldflags -X main.version.
var Version = "0.0.0-dev"

type flagRule struct {
	Default    bool     `yaml:"default"`
	Variant    string   `yaml:"variant"`
	Percent    int      `yaml:"percent"`
	EnabledFor []string `yaml:"enabled_for"`
}

type Module struct {
	featureflagsv1.UnimplementedFeatureFlagsServiceServer
	mu          sync.RWMutex
	flags       map[string]flagRule
	filePath    string
	lastLoadErr error
	grpcSrv     *grpc.Server
	grpcLis     net.Listener
	httpSrv     *http.Server
	httpLis     net.Listener
	id          string
	grpcAddr    string
	httpAddr    string
	sighupCh    chan os.Signal
}

type Config struct {
	ID       string
	FilePath string
	GRPCAddr string
	HTTPAddr string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "feature-flags-file"
	}
	if cfg.FilePath == "" {
		cfg.FilePath = "flags.yaml"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9402"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9404"
	}
	if v := os.Getenv("FEATURE_FLAGS_FILE"); v != "" {
		cfg.FilePath = v
	}
	if v := os.Getenv("FEATURE_FLAGS_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("FEATURE_FLAGS_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	return &Module{
		id:       cfg.ID,
		filePath: cfg.FilePath,
		grpcAddr: cfg.GRPCAddr,
		httpAddr: cfg.HTTPAddr,
		flags:    make(map[string]flagRule),
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:             m.id,
		Name:           "Feature Flags File",
		Version:        Version,
		Roles:          []string{"infrastructure"},
		Description:    "YAML file-backed feature flag provider with SIGHUP reload",
		Author:         "MuxCore",
		Capabilities:   []string{contracts.CapabilityFeatureFlags, "settings"},
		MinCoreVersion: "0.5.0",
		HTTPAddr:       m.httpAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	if err := m.loadFile(); err != nil {
		slog.Warn("feature-flags: could not load initial file", "path", m.filePath, "error", err)
	}
	var err error
	m.grpcLis, err = net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.grpcAddr = m.grpcLis.Addr().String()
	m.httpLis, err = net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen HTTP %s: %w", m.httpAddr, err)
	}
	m.httpAddr = m.httpLis.Addr().String()
	slog.Info("feature-flags initialized", "file", m.filePath, "flags", len(m.flags))
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	featureflagsv1.RegisterFeatureFlagsServiceServer(m.grpcSrv, m)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)
	go func() {
		slog.Info("feature-flags gRPC started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.grpcLis); err != nil {
			slog.Error("feature-flags gRPC error", "error", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", m.serveHealth)
	m.httpSrv = &http.Server{Handler: mux}
	go func() {
		slog.Info("feature-flags HTTP started", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(m.httpLis); err != nil && err != http.ErrServerClosed {
			slog.Error("feature-flags HTTP error", "error", err)
		}
	}()

	sighupCh := make(chan os.Signal, 1)
	m.sighupCh = sighupCh
	signal.Notify(sighupCh, syscall.SIGHUP)
	go func() {
		for range sighupCh {
			slog.Info("SIGHUP: reloading feature flags")
			if err := m.loadFile(); err != nil {
				slog.Error("reload feature flags", "error", err)
			}
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.sighupCh != nil {
		signal.Stop(m.sighupCh)
	}
	if m.httpSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = m.httpSrv.Shutdown(shutdownCtx)
	}
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	slog.Info("feature-flags stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	m.mu.RLock()
	err := m.lastLoadErr
	m.mu.RUnlock()
	return err
}

func (m *Module) serveHealth(w http.ResponseWriter, _ *http.Request) {
	m.mu.RLock()
	loadErr := m.lastLoadErr
	m.mu.RUnlock()

	status := "ok"
	code := http.StatusOK
	if loadErr != nil {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	payload := map[string]string{"status": status}
	if loadErr != nil {
		payload["error"] = loadErr.Error()
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func (m *Module) GRPCListenAddr() string {
	if m.grpcLis != nil {
		return m.grpcLis.Addr().String()
	}
	return m.grpcAddr
}

func (m *Module) HTTPListenAddr() string {
	if m.httpLis != nil {
		return m.httpLis.Addr().String()
	}
	return m.httpAddr
}

func (m *Module) loadFile() error {
	m.mu.RLock()
	path := m.filePath
	m.mu.RUnlock()
	flags, err := readFlagsFile(path)
	if err != nil {
		m.mu.Lock()
		m.lastLoadErr = err
		m.mu.Unlock()
		return err
	}
	m.mu.Lock()
	m.flags = flags
	m.lastLoadErr = nil
	m.mu.Unlock()
	slog.Info("feature-flags loaded", "path", path, "count", len(flags))
	return nil
}

func (m *Module) writeFlagsFile(data []byte) error {
	m.mu.RLock()
	path := m.filePath
	m.mu.RUnlock()
	if err := validateFlagsPath(path); err != nil {
		return err
	}
	flags, err := parseFlagsYAML(data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write flags file: %w", err)
	}
	m.mu.Lock()
	m.flags = flags
	m.lastLoadErr = nil
	m.mu.Unlock()
	slog.Info("feature-flags updated from settings", "path", path, "count", len(flags))
	return nil
}

func readFlagsFile(path string) (map[string]flagRule, error) {
	if err := validateFlagsPath(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read flags file: %w", err)
	}
	return parseFlagsYAML(data)
}

func parseFlagsYAML(data []byte) (map[string]flagRule, error) {
	var flags map[string]flagRule
	if err := yaml.Unmarshal(data, &flags); err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}
	if flags == nil {
		flags = make(map[string]flagRule)
	}
	for name, rule := range flags {
		if rule.Percent < 0 || rule.Percent > 100 {
			return nil, fmt.Errorf("flag %q: percent must be 0-100", name)
		}
	}
	return flags, nil
}

func (m *Module) flagsYAML() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.flags) == 0 && m.lastLoadErr != nil {
		return "", m.lastLoadErr
	}
	out, err := yaml.Marshal(m.flags)
	if err != nil {
		return "", fmt.Errorf("marshal flags: %w", err)
	}
	return string(out), nil
}

func (m *Module) tryLoadPath(path string) (map[string]flagRule, error) {
	if err := validateFlagsPath(path); err != nil {
		return nil, err
	}
	return readFlagsFile(path)
}

func (m *Module) IsEnabled(ctx context.Context, req *featureflagsv1.IsEnabledRequest) (*featureflagsv1.IsEnabledResponse, error) {
	m.mu.RLock()
	rule, ok := m.flags[req.GetFlag()]
	m.mu.RUnlock()
	if !ok {
		return &featureflagsv1.IsEnabledResponse{Enabled: req.GetDefaultValue()}, nil
	}
	callerID := callerIDFromContext(ctx)
	enabled := flagEnabledForCaller(rule, callerID, req.GetFlag())
	return &featureflagsv1.IsEnabledResponse{Enabled: enabled}, nil
}

func (m *Module) GetVariant(ctx context.Context, req *featureflagsv1.GetVariantRequest) (*featureflagsv1.GetVariantResponse, error) {
	m.mu.RLock()
	rule, ok := m.flags[req.GetFlag()]
	m.mu.RUnlock()
	if !ok {
		return &featureflagsv1.GetVariantResponse{Variant: req.GetDefaultValue()}, nil
	}
	callerID := callerIDFromContext(ctx)
	if !flagEnabledForCaller(rule, callerID, req.GetFlag()) {
		return &featureflagsv1.GetVariantResponse{Variant: req.GetDefaultValue()}, nil
	}
	if rule.Variant == "" {
		return &featureflagsv1.GetVariantResponse{Variant: req.GetDefaultValue()}, nil
	}
	return &featureflagsv1.GetVariantResponse{Variant: rule.Variant}, nil
}
