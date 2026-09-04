GO ?= go
GOLANGCI_LINT ?= golangci-lint
NILAWAY ?= nilaway

DIRECT_DEPS_TEMPLATE := {{if and (not .Main) (not .Indirect) (not .Replace)}}{{.Path}}{{end}}

.DEFAULT_GOAL := check

TESTDATA := generate/http/testdata

.PHONY: deps-update tidy fmt

deps-update:
	@deps="$$(GOWORK=off $(GO) list -m -f '$(DIRECT_DEPS_TEMPLATE)' all)"; \
	if [ -n "$$deps" ]; then GOWORK=off $(GO) get -u $$deps; fi
	GOWORK=off $(GO) mod tidy

tidy:
	GOWORK=off $(GO) mod tidy

fmt:
	$(GO) fmt ./...
	$(GOLANGCI_LINT) fmt --no-config --enable gofmt --enable goimports

# Compile the test fixtures into committed descriptor sets. Only this target
# needs buf; `go test` runs against the committed *.pb files.
#
# buf resolves dependencies from $(TESTDATA)/buf.lock (no vendored protos),
# emits a FileDescriptorSet that bundles every import (so protogen can resolve
# them), and keeps source info by default so proto comments survive into the
# generated Go doc comments and Swagger @Description lines.
.PHONY: testdata
testdata:
	@mkdir -p $(TESTDATA)/pb
	@for p in $(TESTDATA)/proto/*.proto; do \
		name=$$(basename $$p .proto); \
		echo "building $$p -> $(TESTDATA)/pb/$$name.pb"; \
		buf build $(TESTDATA) --path $$p --as-file-descriptor-set \
			-o $(TESTDATA)/pb/$$name.pb || exit 1; \
	done

.PHONY: update-golden
# Scoped to the http package: it is the only one that defines -update-golden, so
# passing the flag to ./generate/... would fail the parser/template test binaries.
update-golden: testdata
	$(GO) test ./generate/http/ -run TestGolden -update-golden

.PHONY: build test
build:
	$(GO) build ./...

test: testdata
	$(GO) test ./...

.PHONY: lint check install
lint:
	$(GOLANGCI_LINT) fmt --no-config --enable gofmt --enable goimports --diff
	$(GO) vet ./...
	$(GOLANGCI_LINT) run --no-config
	$(NILAWAY) -include-pkgs="$$($(GO) list -m)" ./...

check:
	GOWORK=off $(GO) mod tidy -diff
	$(MAKE) lint
	$(MAKE) test

install:
	$(GO) install .
