package main

import (
	"flag"
	"fmt"

	"github.com/go-sphere/protoc-gen-sphere/generate/http"
	"github.com/go-sphere/protoc-gen-sphere/generate/template"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

var (
	showVersion = flag.Bool("version", false, "print the version and exit")

	omitempty       = flag.Bool("omitempty", true, "omit if google.api is empty")
	omitemptyPrefix = flag.String("omitempty_prefix", "", "omit if google.api is empty")

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
		fmt.Printf("protoc-gen-sphere %v\n", "0.0.1")
		return
	}
	protogen.Options{
		ParamFunc: flag.CommandLine.Set,
	}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		conf, err := extractConfig()
		if err != nil {
			return err
		}
		err = template.ReplaceTemplateIfNeed(conf.TemplateFile)
		if err != nil {
			return err
		}
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			_, gErr := http.GenerateFile(gen, f, conf)
			if gErr != nil {
				return gErr
			}
		}
		return nil
	})
}

func extractConfig() (*http.Config, error) {
	_routerType, err := http.ParseGoIdent(*routerType)
	if err != nil {
		return nil, err
	}
	_contextType, err := http.ParseGoIdent(*contextType)
	if err != nil {
		return nil, err
	}
	_handlerType, err := http.ParseGoIdent(*handlerType)
	if err != nil {
		return nil, err
	}
	_errorRespType, err := http.ParseGoIdent(*errorRespType)
	if err != nil {
		return nil, err
	}
	_dataRespType, err := http.ParseGoIdent(*dataRespType)
	if err != nil {
		return nil, err
	}

	_serverHandlerFunc, err := http.ParseGoIdent(*serverHandlerFunc)
	if err != nil {
		return nil, err
	}

	conf := &http.Config{
		Omitempty:       *omitempty,
		OmitemptyPrefix: *omitemptyPrefix,

		SwaggerAuth:  *swaggerAuthHeader,
		TemplateFile: *templateFile,

		RouterType:    _routerType,
		ContextType:   _contextType,
		HandlerType:   _handlerType,
		ErrorRespType: _errorRespType,
		DataRespType:  _dataRespType,

		ServerHandlerFunc: _serverHandlerFunc,
		ContextLoadFunc:   *contextLoadFunc,
	}
	return conf, nil
}
