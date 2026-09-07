package parser

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Path pattern regex compiled once at package initialization
	complexLiteralRegex = regexp.MustCompile(`\{([^}=]+)=([^}*]+)/(\*+)\}`)
	literalRegex        = regexp.MustCompile(`\{([^}=]+)=([^}*/]+)\}`)
	doubleWildcardRegex = regexp.MustCompile(`\{([^}=]+)=\*\*\}`)
	singleWildcardRegex = regexp.MustCompile(`\{([^}=]+)=\*\}`)
	simpleParamRegex    = regexp.MustCompile(`\{([^}=]+)\}`)
	multipleSlashRegex  = regexp.MustCompile(`/+`)
	// namedParamRegex matches gin-style ':name' parameters at the start of a
	// path segment only. A colon preceded by another character — the
	// google.api.http custom-method suffix in '/reports:generate' — is a
	// literal part of the URL, not a parameter.
	namedParamRegex      = regexp.MustCompile(`(^|/):([a-zA-Z_][a-zA-Z0-9_]*)`)
	wildcardParamRegex   = regexp.MustCompile(`(^|/)\*([a-zA-Z_][a-zA-Z0-9_]*)`)
	nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_]`)
)

func HTTPRoute(protoPath string) (string, error) {
	if protoPath == "" {
		return "", fmt.Errorf("proto path cannot be empty")
	}
	var nestedName string
	noteNested := func(name string) {
		if nestedName == "" && strings.Contains(name, ".") {
			nestedName = name
		}
	}
	result := protoPath
	// 1.  {param=literal/*} or {param=literal/**}
	result = complexLiteralRegex.ReplaceAllStringFunc(result, func(match string) string {
		matches := complexLiteralRegex.FindStringSubmatch(match)
		if len(matches) >= 4 {
			noteNested(matches[1])
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
			noteNested(matches[1])
			paramName := cleanParamName(matches[1])
			return "/*" + paramName
		}
		return match
	})
	// 4.  {param=*} -> /:param
	result = singleWildcardRegex.ReplaceAllStringFunc(result, func(match string) string {
		matches := singleWildcardRegex.FindStringSubmatch(match)
		if len(matches) >= 2 {
			noteNested(matches[1])
			paramName := cleanParamName(matches[1])
			return "/:" + paramName
		}
		return match
	})
	// 5.  {param} -> /:param
	result = simpleParamRegex.ReplaceAllStringFunc(result, func(match string) string {
		matches := simpleParamRegex.FindStringSubmatch(match)
		if len(matches) >= 2 {
			noteNested(matches[1])
			paramName := cleanParamName(matches[1])
			return "/:" + paramName
		}
		return match
	})
	if nestedName != "" {
		return "", fmt.Errorf("nested path variables such as {%s} are not supported; declare a top-level field and mark it BINDING_LOCATION_URI", nestedName)
	}
	result = multipleSlashRegex.ReplaceAllString(result, "/")
	if !strings.HasPrefix(result, "/") {
		result = "/" + result
	}
	if len(result) > 1 && strings.HasSuffix(result, "/") {
		result = strings.TrimSuffix(result, "/")
	}

	return result, nil
}

func HTTPRouteToSwaggerRoute(ginPath string) string {
	//  :params -> {params}
	swaggerPath := namedParamRegex.ReplaceAllString(ginPath, "${1}{$2}")
	//  *filepath -> {filepath}
	swaggerPath = wildcardParamRegex.ReplaceAllString(swaggerPath, "${1}{$2}")
	return swaggerPath
}

// MidSegmentColon reports whether the route contains a ':' that does not start
// a path segment — the google.api.http custom-method style ('/reports:generate').
// Such colons are literals. gin-backed routers cannot register two different
// literal-colon routes sharing the same path prefix (the tree panics), so the
// generator warns instead of failing.
func MidSegmentColon(route string) bool {
	for i := 1; i < len(route); i++ {
		if route[i] == ':' && route[i-1] != '/' {
			return true
		}
	}
	return false
}

func cleanParamName(paramName string) string {
	cleaned := strings.ReplaceAll(paramName, ".", "_")
	cleaned = nonAlphanumericRegex.ReplaceAllString(cleaned, "_")
	return cleaned
}
