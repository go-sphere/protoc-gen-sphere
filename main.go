package main

import (
	"flag"
	"fmt"

	"github.com/go-sphere/protoc-gen-sphere/generate/http"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

const version = "0.0.1"

var (
	showVersion = flag.Bool("version", false, "print the version and exit")

	omitEmpty       = flag.Bool("omitempty", http.DefaultOmitEmpty, "omit if google.api is empty")
	omitEmptyPrefix = flag.String("omitempty_prefix", http.DefaultOmitEmptyPrefix, "omit if google.api is empty")
	failOnWarn      = flag.Bool("fail_on_warn", http.DefaultFailOnWarn, "treat generation warnings (streaming skips, GET/DELETE body, missing body) as hard errors")

	templateFile      = flag.String("template_file", "", "template file, if not set, use default template")
	swaggerAuthHeader = flag.String("swagger_auth_header", http.DefaultSwaggerAuthHeader, "swagger auth header")

	routerType      = flag.String("router_type", http.DefaultRouterType, "router type")
	contextType     = flag.String("context_type", http.DefaultContextType, "context type")
	handlerType     = flag.String("handler_type", http.DefaultHandlerType, "handler type")
	contextLoadFunc = flag.String("context_load_func", http.DefaultContextLoadFunc, "context load func")

	errorRespType     = flag.String("error_resp_type", http.DefaultErrorRespType, "error response type")
	dataRespType      = flag.String("data_resp_type", http.DefaultDataRespType, "data response type, must support generic")
	serverHandlerFunc = flag.String("server_handler_func", http.DefaultServerHandlerFunc, "server handler func, must support generic")
)

func main() {
	flag.Parse()
	if *showVersion {
		fmt.Printf("protoc-gen-sphere %s\n", version)
		return
	}
	protogen.Options{
		ParamFunc: flag.CommandLine.Set,
	}.Run(run)
}

func run(plugin *protogen.Plugin) error {
	plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
	cfg, err := extractConfig()
	if err != nil {
		return err
	}
	generator, err := http.NewGenerator(cfg)
	if err != nil {
		return err
	}
	for _, file := range plugin.Files {
		if !file.Generate {
			continue
		}
		if _, err := generator.GenerateFile(plugin, file); err != nil {
			return err
		}
	}
	return nil
}

func extractConfig() (*http.Config, error) {
	parsedRouterType, err := http.ParseGoIdent(*routerType)
	if err != nil {
		return nil, err
	}
	parsedContextType, err := http.ParseGoIdent(*contextType)
	if err != nil {
		return nil, err
	}
	parsedHandlerType, err := http.ParseGoIdent(*handlerType)
	if err != nil {
		return nil, err
	}
	parsedErrorRespType, err := http.ParseGoIdent(*errorRespType)
	if err != nil {
		return nil, err
	}
	parsedDataRespType, err := http.ParseGoIdent(*dataRespType)
	if err != nil {
		return nil, err
	}

	parsedServerHandlerFunc, err := http.ParseGoIdent(*serverHandlerFunc)
	if err != nil {
		return nil, err
	}

	cfg := http.DefaultConfig()
	cfg.OmitEmpty = *omitEmpty
	cfg.OmitEmptyPrefix = *omitEmptyPrefix
	cfg.FailOnWarn = *failOnWarn
	cfg.SwaggerAuth = *swaggerAuthHeader
	cfg.TemplateFile = *templateFile
	cfg.RouterType = parsedRouterType
	cfg.ContextType = parsedContextType
	cfg.HandlerType = parsedHandlerType
	cfg.ErrorRespType = parsedErrorRespType
	cfg.DataRespType = parsedDataRespType
	cfg.ServerHandlerFunc = parsedServerHandlerFunc
	cfg.ContextLoadFunc = *contextLoadFunc
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}
