package test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	featureflagsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/featureflags/v1"
	"github.com/Muxcore-Media/feature-flags-file/internal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestFeatureFlagsServiceGRPC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flags.yaml")
	if err := os.WriteFile(path, []byte(`
grpc-flag:
  default: true
  variant: "v2"
`), 0600); err != nil {
		t.Fatal(err)
	}

	mod := internal.NewModule(internal.Config{
		FilePath: path,
		GRPCAddr: "127.0.0.1:0",
		HTTPAddr: "127.0.0.1:0",
	})
	ctx := context.Background()
	if err := mod.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := mod.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = mod.Stop(ctx) })

	conn, err := grpc.NewClient(mod.GRPCListenAddr(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := featureflagsv1.NewFeatureFlagsServiceClient(conn)
	rpcCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-caller-id", "test-client"))

	en, err := client.IsEnabled(rpcCtx, &featureflagsv1.IsEnabledRequest{
		Flag:         "grpc-flag",
		DefaultValue: false,
	})
	if err != nil {
		t.Fatalf("IsEnabled: %v", err)
	}
	if !en.GetEnabled() {
		t.Fatal("grpc-flag should be enabled")
	}

	variant, err := client.GetVariant(rpcCtx, &featureflagsv1.GetVariantRequest{
		Flag:         "grpc-flag",
		DefaultValue: "control",
	})
	if err != nil {
		t.Fatalf("GetVariant: %v", err)
	}
	if variant.GetVariant() != "v2" {
		t.Fatalf("variant = %q want v2", variant.GetVariant())
	}

	missing, err := client.IsEnabled(rpcCtx, &featureflagsv1.IsEnabledRequest{
		Flag:         "missing",
		DefaultValue: true,
	})
	if err != nil {
		t.Fatalf("IsEnabled missing: %v", err)
	}
	if !missing.GetEnabled() {
		t.Fatal("missing flag should use default_value")
	}
}

func TestFeatureFlagsServiceDialRequiresListener(t *testing.T) {
	mod := internal.NewModule(internal.Config{
		FilePath: filepath.Join(t.TempDir(), "flags.yaml"),
		GRPCAddr: "127.0.0.1:0",
		HTTPAddr: "127.0.0.1:0",
	})
	ctx := context.Background()
	if err := mod.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	addr := mod.GRPCListenAddr()
	if _, _, err := net.SplitHostPort(addr); err != nil {
		t.Fatalf("listen addr = %q: %v", addr, err)
	}
}
