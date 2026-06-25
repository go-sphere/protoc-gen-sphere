MODULE := $(shell go list -m)

TESTDATA := generate/http/testdata

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
	go test ./generate/http/ -run TestGolden -update-golden

.PHONY: test
test: testdata
	go test ./...

.PHONY: lint
lint:
	go fix ./...
	go fmt ./...
	go vet ./...
	go get ./...
	go test ./...
	go mod tidy
	golangci-lint fmt --no-config --enable gofmt,goimports
	golangci-lint run --no-config --fix
	nilaway -include-pkgs="$(MODULE)" ./...

.PHONY: install
install:
	go install .