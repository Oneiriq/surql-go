// Package connection provides the SurrealDB connection surface for surql-go:
// pure-data configuration + credential types, and the runtime DatabaseClient
// that wraps the official SurrealDB Go SDK.
//
// # Configuration
//
// ConnectionConfig describes how to reach a SurrealDB deployment (URL,
// namespace, database, timeouts, retry window, live-query flag) and ships
// with LoadConfigFromEnv / LoadNamedConfigFromEnv helpers for environment
// hydration. Credential kinds are modelled as Root / Namespace / Database /
// Scope credentials plus a JWT TokenAuth, all implementing the Credentials
// interface.
//
// # Runtime client
//
// DatabaseClient wraps *surrealdb.DB and provides Connect / Disconnect, the
// four signin/signup primitives, raw Query + QueryWithVars, the Select /
// Create / Update / Merge / Delete CRUD surface, and a Health probe.
// Connect applies exponential backoff governed by the retry_* fields on
// ConnectionConfig.
//
// Transaction wraps the SDK's interactive transaction (Commit / Rollback /
// Execute) and is obtained via DatabaseClient.Begin.
//
// Session (SurrealDB v3 "multiple sessions") is an additional authenticated
// context multiplexed over a single WebSocket connection, obtained via
// DatabaseClient.Attach. Each session carries its own auth state and selected
// namespace/database and mirrors the client query / CRUD surface plus Use /
// Signin / Authenticate / Invalidate / Detach. Sessions are WebSocket-only;
// Attach returns ErrConnection over HTTP transports.
//
// Bucket (SurrealDB v3 object storage) is a handle for file operations against
// a named storage bucket, obtained via DatabaseClient.Bucket or Session.Bucket.
// Every operation binds the bucket name, key, destination, and payload as
// parameters to the type::file($bucket,$key) constructor — caller values are
// never interpolated into SurrealQL. Put accepts a string or []byte payload
// (a []byte is sent as CBOR bytes).
//
// LiveQuery exposes SurrealDB live-query subscriptions through a Go channel;
// HTTP/HTTPS transports are rejected with ErrValidation because the
// underlying protocol is WebSocket-only.
//
// The API mirrors surql-py/src/surql/connection so consumers that have
// written against the Python port port across one-to-one.
package connection
