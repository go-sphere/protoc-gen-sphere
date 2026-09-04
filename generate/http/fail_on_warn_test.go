package http

import (
	"testing"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/testutil"
	"google.golang.org/protobuf/compiler/protogen"
)

// TestFailOnWarn verifies that FailOnWarn promotes generation warnings into
// errors without rejecting valid form bindings.
func TestFailOnWarn(t *testing.T) {
	load := func(t *testing.T) (*protogen.Plugin, *protogen.File) {
		t.Helper()
		set := testutil.LoadDescriptorSet(t, "testdata/pb/no_body.pb")
		plugin := testutil.MustCreatePlugin(t, set, "no_body.proto")
		file := testutil.FileToGenerate(t, plugin)
		return plugin, file
	}

	t.Run("fail_on_warn promotes to error", func(t *testing.T) {
		plugin, file := load(t)
		cfg := DefaultConfig()
		cfg.FailOnWarn = true
		_, err := GenerateFile(plugin, file, cfg)
		if err == nil {
			t.Fatal("expected GenerateFile to fail when FailOnWarn is enabled, got nil")
		}
		want := "method `NoBodyService.GetItem` body should not be declared. File: `no_body.proto`"
		if got := err.Error(); got != want {
			t.Errorf("error = %q, want %q", got, want)
		}
	})

	t.Run("form bindings do not emit warning under fail_on_warn", func(t *testing.T) {
		set := testutil.LoadDescriptorSet(t, "testdata/pb/binding.pb")
		plugin := testutil.MustCreatePlugin(t, set, "binding.proto")
		file := testutil.FileToGenerate(t, plugin)
		cfg := DefaultConfig()
		cfg.FailOnWarn = true
		if _, err := GenerateFile(plugin, file, cfg); err != nil {
			t.Fatalf("expected form binding to succeed without warning when FailOnWarn is enabled, got: %v", err)
		}
	})
}
