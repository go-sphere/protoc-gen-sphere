// Package swagspec is the synthesizer counterpart of swaggo/swag's parser: it
// models a swag annotation block as a typed intermediate representation,
// validates it against both the swag grammar and OpenAPI 2.0 semantics, and
// renders it deterministically. Rendered blocks round-trip through swag's own
// parser (see the round-trip test), so an invalid block fails here instead of
// surfacing as a broken swagger.json downstream.
package swagspec

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Location is the swag `in` classification of a parameter.
type Location string

const (
	LocationQuery    Location = "query"
	LocationPath     Location = "path"
	LocationHeader   Location = "header"
	LocationFormData Location = "formData"
	LocationBody     Location = "body"
)

// bodyCapableMethods lists the router methods on which OpenAPI 2.0 allows
// request payloads (in: body / in: formData). The remaining methods must carry
// all input as query/path/header parameters.
var bodyCapableMethods = map[string]bool{
	"post":  true,
	"put":   true,
	"patch": true,
}

// swaggerPrimitives are the type tokens swag treats as primitive scalars.
var swaggerPrimitives = map[string]bool{
	"string":  true,
	"integer": true,
	"number":  true,
	"boolean": true,
}

var (
	// pathVariable finds {name} variables in a Swagger router path.
	pathVariable = regexp.MustCompile(`\{([^}]+)\}`)
	// routerMethod matches the [verb] part of @Router after lowercasing.
	routerMethod = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// Param is one @Param entry.
type Param struct {
	Name        string
	In          Location
	Type        string
	Required    bool
	Description string
}

// Response is a @Success or @Failure entry. Codes carries every status code
// that shares the schema (swag accepts a comma-separated list plus "default").
type Response struct {
	Codes       []string
	Type        string
	Description string
}

// Operation is the intermediate representation of one swag annotation block.
// It contains no formatting knowledge; Render is the single place that owns
// the swag comment syntax.
type Operation struct {
	Summary     string
	Description string // optional, must be a single line
	Tags        []string
	Accept      string // swag consumes subtype, e.g. "json" or "mpfd"
	Produce     string // swag produces subtype, e.g. "json" or "text/event-stream"
	// RawLines are verbatim extra comment lines (e.g. a plugin-configured
	// auth annotation). They must already start with "//".
	RawLines []string
	Params   []Param
	Success  Response
	Failure  Response
	// RouterPath is the Swagger path with {name} variables.
	RouterPath   string
	RouterMethod string // lowercase HTTP verb, e.g. "get"
}

// Render validates the operation and renders its swag annotation block. The
// result has no trailing newline.
func (o *Operation) Render() (string, error) {
	if err := o.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// @Summary %s\n", o.Summary)
	if o.Description != "" {
		fmt.Fprintf(&b, "// @Description %s\n", o.Description)
	}
	fmt.Fprintf(&b, "// @Tags %s\n", strings.Join(o.Tags, ","))
	fmt.Fprintf(&b, "// @Accept %s\n", o.Accept)
	fmt.Fprintf(&b, "// @Produce %s\n", o.Produce)
	for _, line := range o.RawLines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, param := range o.Params {
		fmt.Fprintf(&b, "// @Param %s %s %s %s %q\n",
			param.Name, param.In, param.Type, strconv.FormatBool(param.Required), param.Description)
	}
	writeResponse := func(attr string, resp Response) {
		fmt.Fprintf(&b, "// @%s %s {object} %s", attr, strings.Join(resp.Codes, ","), resp.Type)
		if resp.Description != "" {
			fmt.Fprintf(&b, " %q", resp.Description)
		}
		b.WriteByte('\n')
	}
	writeResponse("Success", o.Success)
	writeResponse("Failure", o.Failure)
	fmt.Fprintf(&b, "// @Router %s [%s]", o.RouterPath, o.RouterMethod)
	return b.String(), nil
}

// Validate checks the operation against the swag grammar and OpenAPI 2.0
// semantics. Every rule here exists to fail generation early instead of
// emitting an annotation block that swag resolves incorrectly or that misleads
// API clients.
func (o *Operation) Validate() error {
	for _, what := range []struct{ field, value string }{
		{"@Summary", o.Summary},
		{"@Description", o.Description},
		{"@Accept", o.Accept},
		{"@Produce", o.Produce},
		{"@Router path", o.RouterPath},
	} {
		if strings.ContainsAny(what.value, "\n\r") {
			return fmt.Errorf("%s must be a single line", what.field)
		}
	}
	if strings.TrimSpace(o.Summary) == "" {
		return fmt.Errorf("@Summary is required")
	}
	if !strings.HasPrefix(o.RouterPath, "/") {
		return fmt.Errorf("@Router path %q must start with /", o.RouterPath)
	}
	if strings.ContainsAny(o.RouterPath, " \t") {
		return fmt.Errorf("@Router path %q must not contain whitespace", o.RouterPath)
	}
	if !routerMethod.MatchString(o.RouterMethod) {
		return fmt.Errorf("@Router method %q is not a valid verb", o.RouterMethod)
	}
	for i, tag := range o.Tags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("@Tags entry %d is empty", i)
		}
		if strings.ContainsAny(tag, " \t\n\r") {
			return fmt.Errorf("@Tags entry %q must not contain whitespace", tag)
		}
	}
	for _, line := range o.RawLines {
		if !strings.HasPrefix(line, "//") {
			return fmt.Errorf("raw annotation line %q must start with //", line)
		}
		if strings.Contains(line[2:], "\n") {
			return fmt.Errorf("raw annotation line must not contain a newline: %q", line)
		}
	}
	if o.Success.Type == "" || o.Failure.Type == "" {
		return fmt.Errorf("@Success and @Failure require a type")
	}
	if err := validateResponseCodes("Success", o.Success.Codes); err != nil {
		return err
	}
	if err := validateResponseCodes("Failure", o.Failure.Codes); err != nil {
		return err
	}

	seen := make(map[string]bool, len(o.Params))
	pathVars := pathVariablesOf(o.RouterPath)
	bodyCount := 0
	for _, param := range o.Params {
		key := string(param.In) + " " + param.Name
		if seen[key] {
			return fmt.Errorf("duplicate @Param %s %s", param.In, param.Name)
		}
		seen[key] = true
		if err := validateParam(param, o.RouterMethod, o.RouterPath, pathVars); err != nil {
			return err
		}
		if param.In == LocationBody {
			bodyCount++
		}
	}
	if bodyCount > 1 {
		return fmt.Errorf("at most one body @Param is allowed, got %d", bodyCount)
	}
	for _, v := range pathVars {
		if !seen[string(LocationPath)+" "+v] {
			return fmt.Errorf("@Router path variable {%s} has no matching @Param ... path entry", v)
		}
	}
	return nil
}

func validateParam(param Param, routerMethod, routerPath string, pathVars pathVariables) error {
	if param.Name == "" {
		return fmt.Errorf("@Param name is required")
	}
	if strings.ContainsAny(param.Name, " \t\n\r") {
		return fmt.Errorf("@Param name %q must not contain whitespace", param.Name)
	}
	if param.Type == "" {
		return fmt.Errorf("@Param %s: type is required", param.Name)
	}
	if strings.ContainsAny(param.Type, " \t\n\r") {
		return fmt.Errorf("@Param %s: type %q must not contain whitespace", param.Name, param.Type)
	}
	if err := validateSingleLineQuoted("@Param "+param.Name+" description", param.Description); err != nil {
		return err
	}
	switch param.In {
	case LocationPath:
		if !pathVars.has(param.Name) {
			return fmt.Errorf("@Param %s path is not present in @Router path %s", param.Name, routerPath)
		}
		if !param.Required {
			return fmt.Errorf("@Param %s path must be required (OpenAPI 2.0)", param.Name)
		}
		if err := rejectCompositeType(param.Name, param.Type); err != nil {
			return err
		}
	case LocationQuery, LocationHeader, LocationFormData:
		if strings.HasPrefix(param.Type, "map[") {
			return fmt.Errorf("@Param %s %s: map types have no single-token form", param.Name, param.In)
		}
		if elem, ok := strings.CutPrefix(param.Type, "[]"); ok && !swaggerPrimitives[elem] {
			return fmt.Errorf("@Param %s %s: array of %q is not supported, only arrays of primitives are", param.Name, param.In, elem)
		}
	}
	if param.In == LocationFormData && !bodyCapableMethods[routerMethod] {
		return fmt.Errorf("@Param %s formData is not allowed on [%s]; document it as query instead", param.Name, routerMethod)
	}
	if param.In == LocationBody && !bodyCapableMethods[routerMethod] {
		return fmt.Errorf("@Param request body is not allowed on [%s]", routerMethod)
	}
	return nil
}

func validateResponseCodes(attr string, codes []string) error {
	if len(codes) == 0 {
		return fmt.Errorf("@%s requires at least one status code", attr)
	}
	for _, code := range codes {
		if code == "default" {
			continue
		}
		if _, err := strconv.Atoi(code); err != nil {
			return fmt.Errorf("@%s status code %q is neither numeric nor \"default\"", attr, code)
		}
	}
	return nil
}

// rejectCompositeType rejects array and map type tokens: a path/form single
// token can only describe a scalar (or a referenced schema such as an enum).
func rejectCompositeType(name, typ string) error {
	if strings.HasPrefix(typ, "[]") {
		return fmt.Errorf("@Param %s path: array type %q cannot be bound from a single path token", name, typ)
	}
	if strings.HasPrefix(typ, "map[") {
		return fmt.Errorf("@Param %s path: map type %q cannot be bound from a single path token", name, typ)
	}
	return nil
}

// validateSingleLineQuoted rejects content that cannot survive the quoted
// string tail of a @Param/@Success line: swag's grammar closes the description
// at the first double quote and the line at the first newline.
func validateSingleLineQuoted(what, value string) error {
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("%s must be a single line", what)
	}
	if strings.Contains(value, `"`) {
		return fmt.Errorf("%s must not contain double quotes", what)
	}
	return nil
}

type pathVariables []string

// pathVariablesOf extracts the {name} variable names from a Swagger router
// path, in order of appearance.
func pathVariablesOf(routerPath string) pathVariables {
	var names pathVariables
	for _, match := range pathVariable.FindAllStringSubmatch(routerPath, -1) {
		names = append(names, match[1])
	}
	return names
}

func (ps pathVariables) has(name string) bool {
	for _, p := range ps {
		if p == name {
			return true
		}
	}
	return false
}
