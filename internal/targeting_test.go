package internal

import (
	"testing"

	featureflagsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/featureflags/v1"
	"google.golang.org/grpc/metadata"
)

func TestRolloutBucketRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		b := rolloutBucket("module-a", "flag-x")
		if b < 0 || b >= 100 {
			t.Fatalf("bucket out of range: %d", b)
		}
		_ = i
	}
}

func TestFlagEnabledForCallerPercent100(t *testing.T) {
	rule := flagRule{Default: false, Percent: 100}
	if !flagEnabledForCaller(rule, "any-module", "flag") {
		t.Fatal("percent 100 should enable all callers")
	}
}

func TestFlagEnabledForCallerNoIdentity(t *testing.T) {
	rule := flagRule{Default: true, Percent: 50}
	if flagEnabledForCaller(rule, "", "flag") {
		t.Fatal("percent rollout without caller identity should not enable")
	}
}

func TestGetVariantRespectsTargeting(t *testing.T) {
	dir := t.TempDir()
	path := writeFlags(t, dir, `
gated:
  default: false
  variant: "on"
  enabled_for:
    - allowed
`)
	m := NewModule(Config{FilePath: path, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("x-caller-id", "denied"))
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := m.GetVariant(ctx, &featureflagsv1.GetVariantRequest{Flag: "gated", DefaultValue: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if v.GetVariant() != "off" {
		t.Fatalf("variant = %q want off", v.GetVariant())
	}
}

func TestValidateFlagsPath(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "   "} {
		if err := validateFlagsPath(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
	if err := validateFlagsPath("flags.yaml"); err != nil {
		t.Fatal(err)
	}
}
