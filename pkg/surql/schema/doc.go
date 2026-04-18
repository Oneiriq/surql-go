// Package schema provides typed builders for SurrealDB schema definitions.
//
// It is a Go port of surql-py/src/surql/schema/ (fields, table, edge, access
// modules only). Each definition is an immutable value-type struct with:
//
//   - A ToSurql() method that renders the DEFINE statement as SurrealQL.
//   - A Validate() method returning a wrapped surqlerrors.ErrValidation
//     when the definition is malformed.
//
// Typical usage:
//
//	t := schema.NewTable("user",
//	    schema.WithMode(schema.TableModeSchemafull),
//	    schema.WithFields(
//	        schema.StringField("email"),
//	        schema.IntField("age"),
//	    ),
//	    schema.WithIndexes(
//	        schema.UniqueIndex("email_idx", []string{"email"}),
//	    ),
//	)
//	stmts := t.ToSurqlStatements()
//
// The package does NOT include the registry, validator, parser, visualizer,
// or diff tooling; those will be added in later increments.
package schema
