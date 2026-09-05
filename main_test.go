package main

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-sphere/protoc-gen-sphere/generate/http"
)

func TestExtractConfig(t *testing.T) {
	want := http.DefaultConfig()
	got, err := extractConfig()
	if err != nil {
		t.Fatalf("extractConfig() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("extractConfig() = %#v, want %#v", got, want)
	}

	original := *routerType
	t.Cleanup(func() { *routerType = original })
	*routerType = ""
	if _, err := extractConfig(); err == nil {
		t.Fatal("extractConfig() with invalid router_type error = nil")
	}
}

func TestVersionFlag(t *testing.T) {
	binPath := filepath.Join(t.TempDir(), "protoc-gen-sphere")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binPath, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build plugin: %v\n%s", err, output)
	}

	command := exec.CommandContext(t.Context(), binPath, "-version")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run -version: %v\n%s", err, output)
	}
	if got, want := string(output), "protoc-gen-sphere "+version+"\n"; got != want {
		t.Errorf("version output = %q, want %q", got, want)
	}
}
