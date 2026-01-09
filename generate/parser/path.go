package parser

import (
	"fmt"
	"regexp"
	"strings"

	bindingpb "github.com/go-sphere/binding/sphere/binding"

	"google.golang.org/protobuf/compiler/protogen"
)

var (
	// Path pattern regex compiled once at package initialization
	complexLiteralRegex  = regexp.MustCompile(`\{([^}=]+)=([^}*]+)/(\*+)\}`)
	literalRegex         = regexp.MustCompile(`\{([^}=]+)=([^}*/]+)\}`)
	doubleWildcardRegex  = regexp.MustCompile(`\{([^}=]+)=\*\*\}`)
	singleWildcardRegex  = regexp.MustCompile(`\{([^}=]+)=\*\}`)
	simpleParamRegex     = regexp.MustCompile(`\{([^}=]+)\}`)
	multipleSlashRegex   = regexp.MustCompile(`/+`)
	namedParamRegex      = regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)
	wildcardParamRegex   = regexp.MustCompile(`\*([a-zA-Z_][a-zA-Z0-9_]*)`)
	nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_]`)
)

func GinRoute(protoPath string) (string, error) {
	if protoPath == "" {
		return "", fmt.Errorf("proto path cannot be empty")
	}
	result := protoPath
	// 1.  {param=literal/*} or {param=literal/**}
	result = complexLiteralRegex.ReplaceAllStringFunc(result, func(match string) string {
		matches := complexLiteralRegex.FindStringSubmatch(match)
		if len(matches) >= 4 {
			paramName := cleanParamName(matches[1])
			literalPart := matches[2]
			wildcardPart := matches[3]

			if wildcardPart == "**" {
				// {path=assets/**} -> /assets/*path
				return "/" + literalPart + "/*" + paramName
			} else {
				// {path=assets/*} -> /assets/:path
				return "/" + literalPart + "/:" + paramName
			}
		}
		return match
	})
	// 2. {param=literal} -> /literal
	result = literalRegex.ReplaceAllStringFunc(result, func(match string) string {
		matches := literalRegex.FindStringSubmatch(match)
		if len(matches) >= 3 {
			return "/" + matches[2]
		}
		return match
	})
	// 3. {param=**} -> /*param
	result = doubleWildcardRegex.ReplaceAllStringFunc(result, func(match string) string {
		matches := doubleWildcardRegex.FindStringSubmatch(match)
		if len(matches) >= 2 {
			paramName := cleanParamName(matches[1])
			return "/*" + paramName
		}
		return match
	})
	// 4.  {param=*} -> /:param
	result = singleWildcardRegex.ReplaceAllStringFunc(result, func(match string) string {
		matches := singleWildcardRegex.FindStringSubmatch(match)
		if len(matches) >= 2 {
			paramName := cleanParamName(matches[1])
			return "/:" + paramName
		}
		return match
	})
	// 5.  {param} -> /:param
	result = simpleParamRegex.ReplaceAllStringFunc(result, func(match string) string {
		matches := simpleParamRegex.FindStringSubmatch(match)
		if len(matches) >= 2 {
			paramName := cleanParamName(matches[1])
			return "/:" + paramName
		}
		return match
	})
	result = multipleSlashRegex.ReplaceAllString(result, "/")
	if !strings.HasPrefix(result, "/") {
		result = "/" + result
	}
	if len(result) > 1 && strings.HasSuffix(result, "/") {
		result = strings.TrimSuffix(result, "/")
	}

	return result, nil
}

type URIParamsField struct {
	Name     string
	Wildcard bool
	Field    *protogen.Field
}

func GinURIParams(m *protogen.Method, route string) ([]URIParamsField, error) {
	var fields []URIParamsField
	params := parseGinRoutePath(route)
	for _, field := range m.Input.Fields {
		name := string(field.Desc.Name())
		wildcard, exist := params[name]
		if exist {
			if checkBindingLocation(m.Input, field, bindingpb.BindingLocation_BINDING_LOCATION_URI) {
				fields = append(fields, URIParamsField{
					Name:     name,
					Wildcard: wildcard,
					Field:    field,
				})
			} else {
				return nil, fmt.Errorf("method `%s.%s` parameter `%s` is not bound to URI, but it is used in route `%s`. File: `%s`, Field: `%s`",
					m.Parent.Desc.Name(),
					m.Desc.Name(),
					name,
					route,
					m.Parent.Location.SourceFile,
					m.Input.Desc.Name(),
				)
			}
		}
	}
	return fields, nil
}

func parseGinRoutePath(route string) map[string]bool {
	params := make(map[string]bool)
	// :param
	namedMatches := namedParamRegex.FindAllStringSubmatch(route, -1)
	for _, match := range namedMatches {
		if len(match) > 1 {
			params[match[1]] = false
		}
	}
	// *param
	wildcardMatches := wildcardParamRegex.FindAllStringSubmatch(route, -1)
	for _, match := range wildcardMatches {
		if len(match) > 1 {
			params[match[1]] = true
		}
	}
	return params
}

func cleanParamName(paramName string) string {
	cleaned := strings.ReplaceAll(paramName, ".", "_")
	cleaned = nonAlphanumericRegex.ReplaceAllString(cleaned, "_")
	return cleaned
}
