module github.com/go-sphere/protoc-gen-sphere

go 1.25.5

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20251209175733-2a1774d88802.1
	github.com/go-sphere/binding v0.0.4
	google.golang.org/genproto/googleapis/api v0.0.0-20260126211449-d11affda4bed
	google.golang.org/protobuf v1.36.11
)

require github.com/go-sphere/httpx v0.0.3-beta.1.0.20260303064910-e61f711a549e // indirect
