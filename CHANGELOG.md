# Changelog

## Unreleased

## v0.0.6 (2026-10-08)

### Changed

- **BREAKING**: a request message that contains a real `oneof`, directly or in
  a nested message, fails generation. Generated handlers decode the JSON body
  with `encoding/json`, which ignores oneof fields, so the value was silently
  dropped. Use proto3 `optional` fields instead. Replaces the warning for
  non-JSON locations declared on a oneof.
- **BREAKING**: well-known types (`Timestamp`, `Duration`, `wrapperspb.*Value`)
  bound to `QUERY`, `URI` or `HEADER` fail generation; the httpx decoders never
  read them from a single token, so the parameter was silently dropped. The
  shared `scalar_bindability` table changes with protoc-gen-sphere-binding.
- **BREAKING**: a `FORM` field on `GET`, `HEAD`, `DELETE` or `OPTIONS` fails
  generation; `stdx` does not read form values from the query string. Use
  `QUERY`.
- **BREAKING**: a `body` or `response_body` that is a nested path or not a field
  of the message fails generation instead of generating a handler that panics
  or binds the whole message.
- Generated handlers return a validation failure as
  `httpx.BadRequestError(err)` (400) instead of the raw error (500). Error
  parsers that match `*protovalidate.ValidationError` with `errors.As` still
  see it.
- With `body: "*"` and no JSON-located request field (every field is `URI`,
  `QUERY` or `HEADER`, or the message is empty), generated handlers no longer
  call `BindJSON` and the Swagger docs no longer declare a request body, so an
  empty body is accepted on every adapter.
- Generated handlers that do bind a JSON body accept an empty one (`io.EOF`
  from `BindJSON`) and leave the body fields at their zero values, as
  grpc-gateway does. `fiberx` reports an empty body with an error other than
  `io.EOF`, so it still rejects one.
- With `body: "field"`, every other request field left in the JSON body is
  reported as a warning (an error with `fail_on_warn`): it is never bound.
