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

The behavior of `protoc-gen-sphere` can be customized with the following parameters:

- **`version`**: Print the current plugin version and exit. (Default: `false`)
- **`omitempty`**: Omit file generation if `google.api.http` options are not found. (Default: `true`)
- **`omitempty_prefix`**: A file path prefix. If set, `omitempty` will only apply to files with this prefix. (Default: `""`)
- **`template_file`**: Path to a custom Go template file. If not provided, the default internal template is used.
- **`swagger_auth_header`**: The comment for the authorization header in generated Swagger documentation. (Default: `// @Param Authorization header string false "Bearer token"`)
- **`router_type`**: The fully qualified Go type for the router (e.g., `github.com/gin-gonic/gin;IRouter`). (Default: `github.com/gin-gonic/gin;IRouter`)
- **`context_type`**: The fully qualified Go type for the request context (e.g., `github.com/gin-gonic/gin;Context`). (Default: `github.com/gin-gonic/gin;Context`)
- **`data_resp_type`**: The fully qualified Go type for the data response model, which must support generics. (Default: `github.com/go-sphere/sphere/server/ginx;DataResponse`)
- **`error_resp_type`**: The fully qualified Go type for the error response model. (Default: `github.com/go-sphere/sphere/server/ginx;ErrorResponse`)
- **`server_handler_func`**: The wrapper function for handling server responses. (Default: `github.com/go-sphere/sphere/server/ginx;WithJson`)
- **`parse_header_func`**: The function used to parse header parameters. (Default: `github.com/go-sphere/sphere/server/ginx;ShouldBindHeader`)
- **`parse_json_func`**: The function used to parse JSON request bodies. (Default: `github.com/go-sphere/sphere/server/ginx;ShouldBindJSON`)
- **`parse_uri_func`**: The function used to parse URI parameters. (Default: `github.com/go-sphere/sphere/server/ginx;ShouldBindUri`)
- **`parse_form_func`**: The function used to parse form data/query parameters. (Default: `github.com/go-sphere/sphere/server/ginx;ShouldBindQuery`)


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
// @Success 200 {object} ginx.DataResponse[RunTestResponse]
// @Failure 400,401,403,500,default {object} ginx.ErrorResponse
// @Router /api/test/{path_test1}/second/{path_test2} [post]
func _TestService_RunTest0_HTTP_Handler(srv TestServiceHTTPServer) func(ctx *gin.Context) {
    return ginx.WithJson(func(ctx *gin.Context) (*RunTestResponse, error) {
        var in RunTestRequest
        if err := ginx.ShouldBindJSON(ctx, &in); err != nil {
            return nil, err
        }
        if err := ginx.ShouldBindQuery(ctx, &in); err != nil {
            return nil, err
        }
        if err := ginx.ShouldBindUri(ctx, &in); err != nil {
            return nil, err
        }
        if err := protovalidate.Validate(&in); err != nil {
            return nil, err
        }
        out, err := srv.RunTest(ctx, &in)
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
func RegisterTestServiceHTTPServer(route gin.IRouter, srv TestServiceHTTPServer) {
    r := route.Group("/")
    r.POST("/api/test/:path_test1/second/:path_test2", _TestService_RunTest0_HTTP_Handler(srv))
    r.POST("/api/test/body_path_test", _TestService_BodyPathTest0_HTTP_Handler(srv))
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

```go
func main() {
    r := gin.New()
    
    srv := &testService{}
    sharedv1.RegisterTestServiceHTTPServer(r, srv)
    
    r.Run(":8080")
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

## HTTP Annotations Support

The plugin supports the following Google API HTTP annotations:

- `get`, `post`, `put`, `patch`, `delete`: HTTP methods
- `body`: Specifies the request body field (`*` for entire message)
- `response_body`: Specifies the response body field
- Path parameters: `{field_name}` in the URL path
- Additional bindings: Multiple HTTP rules for the same RPC

## Binding Locations

Fields can be bound to different parts of the HTTP request using sphere binding annotations:

- `BINDING_LOCATION_BODY`: JSON request body (default)
- `BINDING_LOCATION_QUERY`: Query parameters
- `BINDING_LOCATION_URI`: Path parameters
- `BINDING_LOCATION_HEADER`: HTTP headers

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
