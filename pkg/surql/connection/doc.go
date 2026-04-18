// Package connection exposes pure-data types describing how to connect
// and authenticate to SurrealDB: ConnectionConfig (URL / namespace /
// database / timeouts / retries / live-queries flag), NamedConnectionConfig,
// and the four credential kinds (Root / Namespace / Database / Scope)
// plus JWT TokenAuth.
//
// It is a 1:1 port of surql-py/src/surql/connection/{config,auth}.py. The
// runtime DatabaseClient and AuthManager (which wrap the SurrealDB SDK
// and handle signin / signup / live queries) land in a follow-up PR.
package connection
