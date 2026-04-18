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
// LiveQuery exposes SurrealDB live-query subscriptions through a Go channel;
// HTTP/HTTPS transports are rejected with ErrValidation because the
// underlying protocol is WebSocket-only.
//
// The API mirrors surql-py/src/surql/connection so consumers that have
// written against the Python port port across one-to-one.
package connection
