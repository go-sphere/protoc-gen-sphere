package http

import (
	"bytes"
	"io"
	"os"
	"strings"
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

// TestFailOnWarnRouteGrammar covers the warnings for routes outside the path
// grammar httpx promises. They are warnings on purpose: httpx puts no
// restriction on these paths, so the generator reports them and still emits
// the route. FailOnWarn is what turns them into a build failure.
func TestFailOnWarnRouteGrammar(t *testing.T) {
	generate := func(t *testing.T, pbFile, protoName string, failOnWarn bool) (*protogen.GeneratedFile, error) {
		t.Helper()
		set := testutil.LoadDescriptorSet(t, pbFile)
		plugin := testutil.MustCreatePlugin(t, set, protoName)
		file := testutil.FileToGenerate(t, plugin)
		cfg := DefaultConfig()
		cfg.FailOnWarn = failOnWarn
		return GenerateFile(plugin, file, cfg)
	}

	t.Run("non-final wildcard fails under fail_on_warn", func(t *testing.T) {
		_, err := generate(t, "testdata/pb/route_grammar.pb", "route_grammar.proto", true)
		if err == nil {
			t.Fatal("expected GenerateFile to fail on a non-final wildcard, got nil")
		}
		want := "method `GrammarService.NonFinalWildcard` route `/v1/grammar/*path/more` is outside the " +
			"route grammar httpx promises: wildcard `*path` is not the final path segment; every adapter " +
			"panics at registration (httpx.ValidateWildcardPath). File: `route_grammar.proto`"
		if got := err.Error(); got != want {
			t.Errorf("error = %q, want %q", got, want)
		}
	})

	t.Run("custom-method colon fails under fail_on_warn", func(t *testing.T) {
		_, err := generate(t, "testdata/pb/custom_verb.pb", "custom_verb.proto", true)
		if err == nil {
			t.Fatal("expected GenerateFile to fail on a literal colon, got nil")
		}
		got := err.Error()
		for _, want := range []string{
			"method `VerbService.GenerateReport` route `/v1/reports:generate` is outside the route grammar httpx promises",
			"static segment `reports:generate` contains a literal ':'",
			"stdx matches it literally",
			"File: `custom_verb.proto`",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("error %q does not contain %q", got, want)
			}
		}
	})

	t.Run("routes inside the grammar stay quiet", func(t *testing.T) {
		// wildcard.proto holds the two shapes that are inside the grammar and
		// look most like the ones that are not: a trailing `*path` and a
		// single-segment `:name`.
		if _, err := generate(t, "testdata/pb/wildcard.pb", "wildcard.proto", true); err != nil {
			t.Fatalf("expected in-grammar wildcard routes to generate without a warning, got: %v", err)
		}
	})

	t.Run("every violation is reported and the file is still generated", func(t *testing.T) {
		var (
			genFile *protogen.GeneratedFile
			err     error
		)
		stderr := captureStderr(t, func() {
			genFile, err = generate(t, "testdata/pb/route_grammar.pb", "route_grammar.proto", false)
		})
		if err != nil {
			t.Fatalf("GenerateFile returned an error without fail_on_warn: %v", err)
		}
		if genFile == nil {
			t.Fatal("expected a generated file, got nil")
		}

		// One warning per violation, not per method: the two-wildcard route
		// breaks two rules and says so twice.
		if got, want := strings.Count(stderr, "is outside the route grammar httpx promises"), 5; got != want {
			t.Errorf("warning count = %d, want %d\n%s", got, want, stderr)
		}
		for _, want := range []string{
			"route `/v1/grammar/*path/more`",
			"wildcard `*path` is not the final path segment; every adapter panics at registration",
			"route `/v1/grammar/*a/x/*b`",
			"wildcard `*b` is a second wildcard and a route may have only one; every adapter panics at registration",
			"route `/v1/grammar/users/:id:activate`",
			"parameter segment `:id:activate` contains a second ':'",
			"no adapter serves the route as written",
			"route `/v1/grammar/files/*path:archive`",
			"wildcard segment `*path:archive` contains a literal ':'",
			"`path` is unreachable",
		} {
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr does not contain %q\n%s", want, stderr)
			}
		}
	})
}

// captureStderr swaps os.Stderr for a pipe while fn runs. Warnings are written
// there by logWarn, which reads the package variable on each call.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	original := os.Stderr
	os.Stderr = w
	collected := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		collected <- buf.String()
	}()
	defer func() {
		_ = r.Close()
	}()
	func() {
		defer func() {
			os.Stderr = original
			_ = w.Close()
		}()
		fn()
	}()
	return <-collected
}
