package http

import (
	goparser "go/parser"
	"go/token"
	"strings"
	"sync"
	"testing"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/testutil"
)

// TestFormUpload_FailOnWarnConcurrentStress tests form upload service generation
// with FailOnWarn = true across 50 concurrent goroutines under race detection.
func TestFormUpload_FailOnWarnConcurrentStress(t *testing.T) {
	bindingSet := testutil.LoadDescriptorSet(t, "testdata/pb/binding.pb")
	integrationSet := testutil.LoadDescriptorSet(t, "testdata/pb/integration.pb")

	const goroutines = 50
	const iterations = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Alternate between binding.pb and integration.pb
				var set = bindingSet
				var protoName = "binding.proto"
				if (id+i)%2 == 1 {
					set = integrationSet
					protoName = "integration.proto"
				}

				plugin := testutil.MustCreatePlugin(t, set, protoName)
				file := testutil.FileToGenerate(t, plugin)

				cfg := DefaultConfig()
				cfg.FailOnWarn = true

				gFile, err := GenerateFile(plugin, file, cfg)
				if err != nil {
					t.Errorf("goroutine %d iteration %d: GenerateFile failed with FailOnWarn=true: %v", id, i, err)
					return
				}
				if gFile == nil {
					t.Errorf("goroutine %d iteration %d: expected generated file, got nil", id, i)
					return
				}

				raw, err := gFile.Content()
				if err != nil {
					t.Errorf("goroutine %d iteration %d: Content failed: %v", id, i, err)
					return
				}
				src := string(raw)

				// Syntactic validity
				fset := token.NewFileSet()
				if _, err := goparser.ParseFile(fset, "out.go", raw, goparser.AllErrors); err != nil {
					t.Errorf("goroutine %d iteration %d: generated Go code has parse errors: %v", id, i, err)
					return
				}

				// Form upload requirements
				if !strings.Contains(src, "ctx.BindForm(&in)") {
					t.Errorf("goroutine %d iteration %d: missing ctx.BindForm(&in)", id, i)
					return
				}
				if !strings.Contains(src, "// @Accept mpfd") {
					t.Errorf("goroutine %d iteration %d: missing // @Accept mpfd", id, i)
					return
				}
				if !strings.Contains(src, "formData") {
					t.Errorf("goroutine %d iteration %d: missing formData swagger param", id, i)
					return
				}
			}
		}(g)
	}

	wg.Wait()
}

// TestFailOnWarn_ConcurrentNegativeStress verifies that protos with illegitimate
// bodies (such as GET with body in no_body.pb) consistently trigger hard errors
// under FailOnWarn = true across 30 concurrent goroutines under race detection.
func TestFailOnWarn_ConcurrentNegativeStress(t *testing.T) {
	set := testutil.LoadDescriptorSet(t, "testdata/pb/no_body.pb")

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			plugin := testutil.MustCreatePlugin(t, set, "no_body.proto")
			file := testutil.FileToGenerate(t, plugin)

			cfg := DefaultConfig()
			cfg.FailOnWarn = true

			_, err := GenerateFile(plugin, file, cfg)
			if err == nil {
				t.Errorf("goroutine %d: expected error under FailOnWarn=true for no_body.pb, got nil", id)
				return
			}
			if !strings.Contains(err.Error(), "body should not be declared") {
				t.Errorf("goroutine %d: unexpected error message: %v", id, err)
				return
			}
		}(g)
	}

	wg.Wait()
}
