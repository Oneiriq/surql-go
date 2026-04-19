// Package settings is the Go equivalent of surql-py's settings module.
//
// Settings carries the whole surql runtime configuration — an
// Environment label, Debug flag, LogLevel, AppName, Version, a
// MigrationPath, and an embedded connection.ConnectionConfig — and is
// hydrated from four layered sources, each overriding the next:
//
//  1. explicit LoadOption values passed to LoadSettings;
//  2. SURQL_-prefixed environment variables (shared with the
//     connection loader);
//  3. .env files loaded through github.com/joho/godotenv;
//  4. a project-root surql.yaml or surql.toml.
//
// The helpers GetSettings, GetDBConfig, and GetMigrationPath expose a
// lazily-initialised process-global Settings for consumers that want
// the Python-style convenience surface without threading a Settings
// pointer through every call site.
package settings
