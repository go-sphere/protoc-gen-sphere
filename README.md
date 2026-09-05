# protoc-gen-sphere

`protoc-gen-sphere` is a protoc plugin that generates HTTP server code from `.proto` files. It is designed to inspect
service definitions within your protobuf files and automatically generate corresponding HTTP handlers based on Google
API annotations and a specified template. Inspired
by [protoc-gen-go-http](https://github.com/go-kratos/kratos/tree/main/cmd/protoc-gen-go-http).


## Installation

To install `protoc-gen-sphere`, use the following command:

```bash
go install github.com/go-sphere/protoc-gen-sphere@latest
```


## Flags

The behavior of `protoc-gen-sphere` can be customized with the following parameters. Type flags use the
`import/path;Identifier` format.

| Flag                  | Description                                                                                                             | Default                                                     |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------|
| `version`             | Print the current plugin version and exit.                                                                            | `false`                                                     |
| `omitempty`           | Omit file generation for files whose methods have no `google.api.http` option.                                        | `true`                                                      |
| `omitempty_prefix`    | A file path prefix. When set, `omitempty` only applies to files with this prefix.                                     | `""`                                                        |
| `fail_on_warn`        | Treat generation warnings (skipped streaming methods, GET/DELETE declaring a body, missing body) as hard errors.     | `false`                                                     |
| `template_file`       | Path to a custom Go template file. When empty the embedded default template is used.                                 | `""`                                                        |
| `swagger_auth_header` | The comment injected as the authorization header in generated Swagger documentation.                                 | `// @Param Authorization header string false "Bearer token"` |
| `router_type`         | Fully qualified Go type for the router.                                                                               | `github.com/go-sphere/httpx;Router`                         |
| `context_type`        | Fully qualified Go type for the request context.                                                                     | `github.com/go-sphere/httpx;Context`                        |
| `handler_type`        | Fully qualified Go type returned by each generated handler.                                                          | `github.com/go-sphere/httpx;Handler`                        |
| `context_load_func`   | Expression appended to the context value to obtain a `context.Context` (e.g. `ctx.Context()`).                       | `.Context()`                                                |
| `data_resp_type`      | Fully qualified Go type for the data response model; must support generics.                                          | `github.com/go-sphere/sphere/server/httpz;DataResponse`     |
| `error_resp_type`     | Fully qualified Go type for the error response model.                                                                | `github.com/go-sphere/sphere/server/httpz;ErrorResponse`    |
| `server_handler_func` | Wrapper function that adapts the generated handler to the response model; must support generics.                    | `github.com/go-sphere/sphere/server/httpz;WithJson`         |
| `stream_handler_func` | Wrapper function for server-streaming (SSE) handlers; must support generics.                                        | `github.com/go-sphere/sphere/server/httpz;WithSSE`          |
| `stream_type`         | Generic stream type returned by the prepare phase of a streaming handler.                                            | `github.com/go-sphere/sphere/server/httpz;SSEStream`        |

> Request binding no longer uses standalone `parse_*_func` flags. Binding is performed through methods on the
> configured `context_type` (`BindJSON` / `BindQuery` / `BindURI` / `BindHeader` / `BindForm`), so customizing the
> binding behavior is done via `context_type`.


## Usage with Buf

To use `protoc-gen-sphere` with `buf`, you can configure it in your `buf.gen.yaml` file. Here is an example configuration:

```yaml
version: v2
managed:
  enabled: true
  disable:
    - file_option: go_package_prefix
      module: buf.build/googleapis/googleapis
    - file_option: go_package_prefix
      module: buf.build/bufbuild/protovalidate
  override:
    - file_option: go_package_prefix
      value: github.com/go-sphere/sphere-layout/api
plugins:
  - local: protoc-gen-sphere
    out: api
    opt:
      - paths=source_relative
      - swagger_auth_header=// @Security ApiKeyAuth
```

## Prerequisites

You need to have the following dependencies in your project. Add them to your `buf.yaml`:

```yaml
deps:
  - buf.build/googleapis/googleapis
  - buf.build/bufbuild/protovalidate
  - buf.build/go-sphere/binding
```

## Proto Definition Example

Here's how to define services with HTTP annotations in your `.proto` files:

```protobuf
syntax = "proto3";

package shared.v1;

import "buf/validate/validate.proto";
import "google/api/annotations.proto";
import "sphere/binding/binding.proto";

service TestService {
  rpc RunTest(RunTestRequest) returns (RunTestResponse) {
    option (google.api.http) = {
      post: "/api/test/{path_test1}/second/{path_test2}"
      body: "*"
    };
  }

  // test comment line1
  // test comment line2
  // test comment line3
  rpc BodyPathTest(BodyPathTestRequest) returns (BodyPathTestResponse) {
    option (google.api.http) = {
      post: "/api/test/body_path_test"
      body: "request"
      response_body: "response"
    };
  }
}

message RunTestRequest {
  string field_test1 = 1;
  int64 field_test2 = 2;
  string path_test1 = 3 [(sphere.binding.location) = BINDING_LOCATION_URI];
  int64 path_test2 = 4 [(sphere.binding.location) = BINDING_LOCATION_URI];
  string query_test1 = 5 [
    (buf.validate.field).required = true,
    (sphere.binding.location) = BINDING_LOCATION_QUERY
  ];
  int64 query_test2 = 6 [(sphere.binding.location) = BINDING_LOCATION_QUERY];
  optional string optional_query = 7 [(sphere.binding.location) = BINDING_LOCATION_QUERY]; // Will be marked as optional in Swagger
}

message RunTestResponse {
  string field_test1 = 1;
  int64 field_test2 = 2;
  string path_test1 = 3;
  int64 path_test2 = 4;
  string query_test1 = 5;
  int64 query_test2 = 6;
}
```

## Generated Code

The plugin generates Go code with HTTP handlers, route registration, and Swagger documentation. Here's what gets generated:

### HTTP Handler Functions

```go
// @Summary RunTest
// @Tags shared.v1,shared.v1.TestService
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param path_test1 path string true "path_test1"
// @Param path_test2 path integer true "path_test2"
// @Param query_test1 query string true "query_test1"
// @Param query_test2 query integer false "query_test2"
// @Param request body RunTestRequest true "request body"
// @Success 200 {object} httpz.DataResponse[RunTestResponse]
// @Failure 400,401,403,500,default {object} httpz.ErrorResponse
// @Router /api/test/{path_test1}/second/{path_test2} [post]
func _TestService_RunTest0_HTTP_Handler(srv TestServiceHTTPServer) httpx.Handler {
    return httpz.WithJson(func(ctx httpx.Context) (*RunTestResponse, error) {
        var in RunTestRequest
        if err := ctx.BindJSON(&in); err != nil {
            return nil, err
        }
        if err := ctx.BindQuery(&in); err != nil {
            return nil, err
        }
        if err := ctx.BindURI(&in); err != nil {
            return nil, err
        }
        if err := protovalidate.Validate(&in); err != nil {
            return nil, err
        }
        out, err := srv.RunTest(ctx.Context(), &in)
        if err != nil {
            return nil, err
        }
        return out, nil
    })
}
```

### Service Interface

```go
type TestServiceHTTPServer interface {
    // BodyPathTest test comment line1
    // test comment line2
    // test comment line3
    BodyPathTest(context.Context, *BodyPathTestRequest) (*BodyPathTestResponse, error)
    RunTest(context.Context, *RunTestRequest) (*RunTestResponse, error)
}
```

### Route Registration

```go
func RegisterTestServiceHTTPServer(route httpx.Router, srv TestServiceHTTPServer) {
    r := route.Group("/")
    r.Handle("POST", "/api/test/:path_test1/second/:path_test2", _TestService_RunTest0_HTTP_Handler(srv))
    r.Handle("POST", "/api/test/body_path_test", _TestService_BodyPathTest0_HTTP_Handler(srv))
}
```

### Constants and Endpoints

```go
const OperationTestServiceRunTest = "/shared.v1.TestService/RunTest"
const OperationTestServiceBodyPathTest = "/shared.v1.TestService/BodyPathTest"

var EndpointsTestService = [...][3]string{
    {OperationTestServiceRunTest, "POST", "/api/test/:path_test1/second/:path_test2"},
    {OperationTestServiceBodyPathTest, "POST", "/api/test/body_path_test"},
}
```

## Usage in Code

### Implementing the Service

```go
type testService struct {
    // your dependencies
}

func (s *testService) RunTest(ctx context.Context, req *sharedv1.RunTestRequest) (*sharedv1.RunTestResponse, error) {
    // your business logic
    return &sharedv1.RunTestResponse{
        FieldTest1: req.FieldTest1,
        FieldTest2: req.FieldTest2,
        PathTest1:  req.PathTest1,
        PathTest2:  req.PathTest2,
        QueryTest1: req.QueryTest1,
        QueryTest2: req.QueryTest2,
    }, nil
}

func (s *testService) BodyPathTest(ctx context.Context, req *sharedv1.BodyPathTestRequest) (*sharedv1.BodyPathTestResponse, error) {
    // your business logic
    return &sharedv1.BodyPathTestResponse{
        Response: []*sharedv1.BodyPathTestResponse_Response{
            {
                FieldTest1: req.Request.FieldTest1,
                FieldTest2: req.Request.FieldTest2,
            },
        },
    }, nil
}
```

### Registering Routes

The generated `Register...HTTPServer` accepts an `httpx.Router`. Any `httpx.Engine` implementation works; the example
below uses the Gin-backed engine (`github.com/go-sphere/httpx/ginx`). Swap it for `echox`, `fiberx` or `hertzx` to
target another framework.

```go
func main() {
    engine := ginx.New()

    srv := &testService{}
    sharedv1.RegisterTestServiceHTTPServer(engine.Group("/"), srv)

    _ = engine.Start()
}
```

## Features

- **Automatic HTTP Handler Generation**: Creates Gin handlers from protobuf service definitions
- **Request Binding**: Automatically binds JSON body, query parameters, and URI parameters
- **Validation Integration**: Integrates with `protovalidate` for request validation
- **Proto3 Optional Support**: Correctly handles `optional` keyword for query, path, and header parameters in Swagger documentation
- **Swagger Documentation**: Generates Swagger/OpenAPI comments for each endpoint with accurate required/optional field markers
- **Response Body Customization**: Supports custom response body fields via `response_body` option, including nested arrays and maps
- **Flexible Binding**: Works with sphere binding annotations for fine-grained control
- **Error Handling**: Integrates with sphere error handling framework with proper error propagation
- **Route Constants**: Generates operation constants and endpoint arrays for easy reference
- **Server Streaming (SSE)**: `rpc Watch(Req) returns (stream Resp)` generates a Server-Sent Events endpoint

## Server-Streaming Methods

A method declared with a `stream` reply generates an SSE handler instead of a
unary one:

```proto
rpc Chat(ChatRequest) returns (stream ChatResponse) {
  option (google.api.http) = {
    post: "/api/chat"
    body: "*"
  };
}
```

The generated server interface takes a push callback; return `nil` to end the
stream with a terminal `done` event, or an error to end it with an `error`
event (errors before the first send still produce a plain JSON error status):

```go
Chat(ctx context.Context, req *ChatRequest, send func(*ChatResponse) error) error
```

Each sent message is delivered as one SSE `data:` event encoded with the same
JSON encoding as unary responses. Request binding and validation run before
the stream commits, so those failures return regular 4xx JSON errors.

Notes:

- Client-streaming and bidirectional methods have no HTTP/1.1 mapping and are
  skipped with a warning (an error with `fail_on_warn`).
- `response_body` is ignored on streaming methods (warned): events carry whole
  reply messages.
- If the same proto is also compiled with `protoc-gen-go-grpc`, that plugin
  generates a genuine gRPC streaming stub for the method — usually the desired
  behavior, but the two transports are independent.
- To let clients resume with `Last-Event-ID`, bind the header as a regular
  request field (`BINDING_LOCATION_HEADER`); event IDs and resume semantics
  are the service implementation's responsibility.

## HTTP Annotations Support

The plugin supports the following Google API HTTP annotations:

- `get`, `post`, `put`, `patch`, `delete`: HTTP methods
- `body`: Specifies the request body field (`*` for entire message)
- `response_body`: Specifies the response body field
- Path parameters: `{field_name}` in the URL path
- Additional bindings: Multiple HTTP rules for the same RPC

## Binding Locations

Fields can be bound to different parts of the HTTP request using sphere binding annotations:

- `BINDING_LOCATION_JSON`: JSON request body (default)
- `BINDING_LOCATION_QUERY`: Query parameters
- `BINDING_LOCATION_URI`: Path parameters
- `BINDING_LOCATION_HEADER`: HTTP headers
- `BINDING_LOCATION_FORM`: Form / multipart form data

> `QUERY`, `URI` and `HEADER` bind a value from a single string token, so only scalar fields (and the well-known scalar
> wrappers `Timestamp`, `Duration` and `wrapperspb.*Value`) may use them. Marking a `map`, `bytes` or arbitrary
> `message` field with one of these locations is a generation-time error. Use `JSON` (or `FORM` for `bytes`/files)
> instead.

### Optional Fields

The plugin correctly handles proto3 `optional` keyword:

- **Query/Header parameters**: `optional` fields are marked as `required=false` in Swagger (default behavior)
- **Path parameters**: `optional` fields are marked as `required=false` in Swagger (though path params are typically required)
- **Validation integration**: Works seamlessly with `buf.validate` constraints - if a field has `(buf.validate.field).required = true`, it will be marked as required regardless of the `optional` keyword

Example:
```protobuf
optional string optional_field = 1 [(sphere.binding.location) = BINDING_LOCATION_QUERY];
string required_field = 2 [
  (buf.validate.field).required = true,
  (sphere.binding.location) = BINDING_LOCATION_QUERY
];
```

In the generated Swagger documentation:
- `optional_field` → `@Param optional_field query string false "optional_field"`
- `required_field` → `@Param required_field query string true "required_field"`

## Customization Options

All the configuration flags allow you to customize the generated code to work with different frameworks and response types. The default configuration is optimized for the sphere framework with Gin router.
