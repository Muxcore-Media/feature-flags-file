package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/yaml.v3"

	"github.com/Muxcore-Media/core/pkg/contracts"
	featureflagsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/featureflags/v1"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"github.com/Muxcore-Media/feature-flags-file/internal/grpctls"
)

type flagRule struct {
	Default bool   `yaml:"default"`
	Variant string `yaml:"variant"`
}

type Module struct {
	featureflagsv1.UnimplementedFeatureFlagsServiceServer
	mu       sync.RWMutex
	flags    map[string]flagRule
	filePath string
	grpcSrv  *grpc.Server
	grpcLis  net.Listener
	httpLis  net.Listener
	id       string
	grpcAddr string
	httpAddr string
	sighupCh chan os.Signal
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
		cfg.GRPCAddr = "127.0.0.1:9402"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9404"
	}
	if v := os.Getenv("FEATURE_FLAGS_FILE"); v != "" {
		cfg.FilePath = v
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
		Version:        "0.1.3",
		Roles:          []string{"infrastructure"},
		Description:    "YAML/JSON file-backed feature flag provider with SIGHUP reload",
		Author:         "MuxCore",
		Capabilities:   []string{contracts.CapabilityFeatureFlags, "settings"},
		MinCoreVersion: "0.5.0",
		HTTPAddr:       m.grpcAddr,
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
	m.httpLis, err = net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen HTTP %s: %w", m.httpAddr, err)
	}
	slog.Info("feature-flags initialized", "file", m.filePath, "flags", len(m.flags))
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	var grpcOpts []grpc.ServerOption
	tlsCfg, err := grpctls.ServerConfig(m.filePath)
	if err != nil {
		return fmt.Errorf("gRPC TLS: %w", err)
	}
	if tlsCfg != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
		slog.Info("feature-flags-file gRPC TLS enabled", "addr", m.grpcAddr)
	} else {
		slog.Warn("feature-flags-file gRPC listening without TLS (dev only)",
			"addr", m.grpcAddr,
			"hint", "unset MUXCORE_INSECURE_DISABLE_TLS for production",
		)
	}
	m.grpcSrv = grpc.NewServer(grpcOpts...)
	featureflagsv1.RegisterFeatureFlagsServiceServer(m.grpcSrv, m)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)
	go func() {
		slog.Info("feature-flags gRPC started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.grpcLis); err != nil {
			slog.Error("feature-flags gRPC error", "error", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	go func() {
		slog.Info("feature-flags HTTP started", "addr", m.httpAddr)
		_ = http.Serve(m.httpLis, mux)
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
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	slog.Info("feature-flags stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}

func (m *Module) loadFile() error {
	m.mu.RLock()
	path := m.filePath
	m.mu.RUnlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read flags file: %w", err)
	}
	var flags map[string]flagRule
	if err := yaml.Unmarshal(data, &flags); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	m.mu.Lock()
	m.flags = flags
	m.mu.Unlock()
	slog.Info("feature-flags loaded", "path", path, "count", len(flags))
	return nil
}

func (m *Module) IsEnabled(ctx context.Context, req *featureflagsv1.IsEnabledRequest) (*featureflagsv1.IsEnabledResponse, error) {
	m.mu.RLock()
	rule, ok := m.flags[req.GetFlag()]
	m.mu.RUnlock()
	if !ok {
		return &featureflagsv1.IsEnabledResponse{Enabled: req.GetDefaultValue()}, nil
	}
	return &featureflagsv1.IsEnabledResponse{Enabled: rule.Default}, nil
}

func (m *Module) GetVariant(ctx context.Context, req *featureflagsv1.GetVariantRequest) (*featureflagsv1.GetVariantResponse, error) {
	m.mu.RLock()
	rule, ok := m.flags[req.GetFlag()]
	m.mu.RUnlock()
	if !ok || rule.Variant == "" {
		return &featureflagsv1.GetVariantResponse{Variant: req.GetDefaultValue()}, nil
	}
	return &featureflagsv1.GetVariantResponse{Variant: rule.Variant}, nil
}
