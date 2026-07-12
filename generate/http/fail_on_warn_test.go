package http

import (
	"testing"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/testutil"
	"google.golang.org/protobuf/compiler/protogen"
)

// TestFailOnWarn verifies ENC-25: with FailOnWarn disabled (the default) a
// warning-triggering proto still generates, while enabling FailOnWarn promotes
// the warning into a hard error so `buf generate` fails. The binding fixture's
// Upload RPC is a POST that declares no body, which emits the "body is not
// declared" warning.
func TestFailOnWarn(t *testing.T) {
	load := func(t *testing.T) (*protogen.Plugin, *protogen.File) {
		t.Helper()
		set := testutil.LoadDescriptorSet(t, "testdata/pb/binding.pb")
		plugin := testutil.MustCreatePlugin(t, set, "binding.proto")
		file := testutil.FileToGenerate(t, plugin)
		return plugin, file
	}

	t.Run("default keeps generating", func(t *testing.T) {
		plugin, file := load(t)
		cfg := DefaultConfig()
		if cfg.FailOnWarn {
			t.Fatal("FailOnWarn must default to false")
		}
		if _, err := GenerateFile(plugin, file, cfg); err != nil {
			t.Fatalf("expected generation to succeed with warnings, got: %v", err)
		}
	})

	t.Run("fail_on_warn promotes to error", func(t *testing.T) {
		plugin, file := load(t)
		cfg := DefaultConfig()
		cfg.FailOnWarn = true
		if _, err := GenerateFile(plugin, file, cfg); err == nil {
			t.Fatal("expected GenerateFile to fail when FailOnWarn is enabled, got nil")
		}
	})
}
