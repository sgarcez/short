# short

`short` is a URL shortening example service using [go-kit](https://github.com/go-kit/kit).

It supports HTTP/REST and gRPC transports, instrumentation and logging, rate limiting and circuit breaking.

Instrumentation and logging is implemented at service and endpoint levels.

A client is provided following the client library pattern. It includes client side rate limiting and circuit breaking.

A simple in memory map is the only implemented storage backend.

The server binary runs HTTP and gRPC servers concurrently. The client binary also supports both transports.

## Functional requirements

- Given a value the service should generate a short unique key for it.
- When a key is looked up the corresponding value should be returned.
- Created keys should be URL safe.

### Potential future requirements

- Custom keys.
- Entries can have TTLs.
- Metadata (creation time, access count, user id)

This service is expected to live in a modern microservices environment and as
such the following assumptions were made:

- Interaction is made exclusively via APIs, there is no user facing HTTP redirection. This would be provided by a service closer to the user.
- Authentication and authorisation would happen in a calling service and/or mesh sidecar.
- Network tracing would happen in a mesh sidecar.
- Rate limiting/Circuit breaking protection should maybe happen in a mesh sidecar although simple in-app implementations are included.
- In an attempt to be slightly more general purpose the service allows any string (up to a maxLen) to be used as a value and does not perform URL specific validation. A calling service could enforce URL validation if required. In any case the created keys are URL safe.

## Design

Keys are short substrings of MD5 hashed input values, accounting for substring collisions by attempting different substring patterns.
In theory this should allow for fully deterministic database reconstitution from logs. In practice the replay would have to account for:

- Errors
- Changes in algorithm, min key size, etc
- Key TTLs

## Binaries

The server binary is available in cmd/shortsvc. The client binary is available in cmd/shortcli.

## Trying it out

Run the server

```
$go run shortsvc.go
```

HTTP client create and lookup

```
go run shortcli.go -http-addr=:8081 -method=create 12345 
gnzLDu

go run shortcli.go -http-addr=:8081 -method=lookup gnzLDu
```

gRPC client lookup

```
go run shortcli.go -grpc-addr=:8082 -method=lookup gnzLDu
```
