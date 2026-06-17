package schema

import (
	"strconv"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// A SurrealDB full-text index references an analyzer that turns stored text
// and query text into comparable tokens — the lexical side of hybrid
// (sparse + dense) retrieval. An analyzer is a tokenizer chain (how the text
// is split) followed by a filter chain (how each token is normalised).
//
// This file renders the DEFINE ANALYZER statement from a typed
// AnalyzerDefinition, so callers define the analyzer in code rather than
// hand-authoring SurrealQL — exactly as TableDefinition does for tables. Pair
// it with a BM25 SearchIndex (see BM25Index) and the
// query.Query.FullTextSearch builder for end-to-end lexical recall.

// Tokenizer splits text into terms before the filter chain runs.
//
// It renders as the lowercase SurrealQL keyword used inside the
// TOKENIZERS ... clause.
type Tokenizer string

// Tokenizer values.
const (
	// TokenizerBlank splits on whitespace (`blank`).
	TokenizerBlank Tokenizer = "blank"
	// TokenizerCamel splits on case transitions (`camelCase` -> `camel`, `Case`).
	TokenizerCamel Tokenizer = "camel"
	// TokenizerClass splits on Unicode character-class transitions — letters,
	// digits, and punctuation become separate tokens (`class`). The
	// general-purpose default for prose and identifiers.
	TokenizerClass Tokenizer = "class"
	// TokenizerPunct splits on punctuation (`punct`).
	TokenizerPunct Tokenizer = "punct"
)

// IsValid reports whether the Tokenizer is recognised.
func (t Tokenizer) IsValid() bool {
	switch t {
	case TokenizerBlank, TokenizerCamel, TokenizerClass, TokenizerPunct:
		return true
	}
	return false
}

// String renders the tokenizer as its SurrealQL keyword.
func (t Tokenizer) String() string { return string(t) }

// TokenFilterKind tags the variant of a TokenFilter.
type TokenFilterKind string

// TokenFilterKind values.
const (
	// FilterASCII folds accented / Unicode characters to their nearest ASCII
	// equivalent (`ascii`).
	FilterASCII TokenFilterKind = "ascii"
	// FilterLowercase lowercases every token (`lowercase`).
	FilterLowercase TokenFilterKind = "lowercase"
	// FilterUppercase uppercases every token (`uppercase`).
	FilterUppercase TokenFilterKind = "uppercase"
	// FilterEdgeNgram emits edge n-grams (prefixes) of length min..=max for
	// prefix / typeahead matching (`edgengram(min,max)`).
	FilterEdgeNgram TokenFilterKind = "edgengram"
	// FilterNgram emits n-grams of length min..=max (`ngram(min,max)`).
	FilterNgram TokenFilterKind = "ngram"
	// FilterSnowball reduces each token to its Snowball stem for the given
	// language, e.g. `snowball(english)` — improves recall by matching word
	// variants.
	FilterSnowball TokenFilterKind = "snowball"
)

// TokenFilter normalises or expands each token after tokenization.
//
// Filters run in declaration order; each renders as the SurrealQL keyword (or
// parameterised call) used inside the FILTERS ... clause. Use the constructor
// helpers (ASCII, Lowercase, Uppercase, EdgeNgram, Ngram, Snowball) rather
// than building the struct directly.
type TokenFilter struct {
	// Kind selects the filter variant.
	Kind TokenFilterKind
	// Min is the lower bound for EdgeNgram / Ngram filters.
	Min int
	// Max is the upper bound for EdgeNgram / Ngram filters.
	Max int
	// Language is the Snowball stemmer language (e.g. "english").
	Language string
}

// ASCII builds an `ascii` token filter.
func ASCII() TokenFilter { return TokenFilter{Kind: FilterASCII} }

// Lowercase builds a `lowercase` token filter.
func Lowercase() TokenFilter { return TokenFilter{Kind: FilterLowercase} }

// Uppercase builds an `uppercase` token filter.
func Uppercase() TokenFilter { return TokenFilter{Kind: FilterUppercase} }

// EdgeNgram builds an `edgengram(min,max)` token filter spanning min..=max.
func EdgeNgram(minLen, maxLen int) TokenFilter {
	return TokenFilter{Kind: FilterEdgeNgram, Min: minLen, Max: maxLen}
}

// Ngram builds an `ngram(min,max)` token filter spanning min..=max.
func Ngram(minLen, maxLen int) TokenFilter {
	return TokenFilter{Kind: FilterNgram, Min: minLen, Max: maxLen}
}

// Snowball builds a `snowball(language)` token filter (e.g. "english").
func Snowball(language string) TokenFilter {
	return TokenFilter{Kind: FilterSnowball, Language: language}
}

// ToSurql renders the filter as its SurrealQL keyword / call.
func (f TokenFilter) ToSurql() string {
	switch f.Kind {
	case FilterASCII, FilterLowercase, FilterUppercase:
		return string(f.Kind)
	case FilterEdgeNgram:
		return "edgengram(" + strconv.Itoa(f.Min) + "," + strconv.Itoa(f.Max) + ")"
	case FilterNgram:
		return "ngram(" + strconv.Itoa(f.Min) + "," + strconv.Itoa(f.Max) + ")"
	case FilterSnowball:
		return "snowball(" + f.Language + ")"
	default:
		return string(f.Kind)
	}
}

// String implements fmt.Stringer, matching ToSurql.
func (f TokenFilter) String() string { return f.ToSurql() }

// AnalyzerDefinition is an immutable DEFINE ANALYZER schema definition: a named
// tokenizer + filter chain referenced by a full-text SearchIndex.
type AnalyzerDefinition struct {
	// Name is the analyzer name (referenced by a full-text index's
	// ANALYZER <name> clause).
	Name string
	// Tokenizers are applied, in order, to split the text.
	Tokenizers []Tokenizer
	// Filters are applied, in order, to normalise each token.
	Filters []TokenFilter
}

// AnalyzerOption customises an AnalyzerDefinition created via Analyzer /
// StandardAnalyzer.
type AnalyzerOption func(*AnalyzerDefinition)

// WithTokenizer appends one tokenizer to the analyzer definition.
func WithTokenizer(tokenizer Tokenizer) AnalyzerOption {
	return func(a *AnalyzerDefinition) { a.Tokenizers = append(a.Tokenizers, tokenizer) }
}

// WithTokenizers appends several tokenizers to the analyzer definition.
func WithTokenizers(tokenizers ...Tokenizer) AnalyzerOption {
	return func(a *AnalyzerDefinition) { a.Tokenizers = append(a.Tokenizers, tokenizers...) }
}

// WithFilter appends one filter to the analyzer definition.
func WithFilter(filter TokenFilter) AnalyzerOption {
	return func(a *AnalyzerDefinition) { a.Filters = append(a.Filters, filter) }
}

// WithFilters appends several filters to the analyzer definition.
func WithFilters(filters ...TokenFilter) AnalyzerOption {
	return func(a *AnalyzerDefinition) { a.Filters = append(a.Filters, filters...) }
}

// Analyzer constructs an AnalyzerDefinition with the given name and options.
// With no options it is an empty analyzer (no tokenizers or filters yet),
// which renders just `DEFINE ANALYZER <name>;`.
func Analyzer(name string, opts ...AnalyzerOption) AnalyzerDefinition {
	a := AnalyzerDefinition{Name: name}
	for _, opt := range opts {
		opt(&a)
	}
	return a
}

// StandardAnalyzer is a sensible general-purpose analyzer for BM25 lexical
// recall: the `class` tokenizer with `lowercase` + `ascii` filters. Apply
// WithFilter(Snowball("english")) for language-specific stemming.
func StandardAnalyzer(name string, opts ...AnalyzerOption) AnalyzerDefinition {
	base := []AnalyzerOption{
		WithTokenizer(TokenizerClass),
		WithFilters(Lowercase(), ASCII()),
	}
	return Analyzer(name, append(base, opts...)...)
}

// Validate checks the analyzer definition.
//
// It returns an ErrValidation error when the name is empty.
func (a AnalyzerDefinition) Validate() error {
	if a.Name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "analyzer name cannot be empty")
	}
	return nil
}

// ToSurql renders the DEFINE ANALYZER statement.
func (a AnalyzerDefinition) ToSurql() string {
	return a.toSurql(false)
}

// ToSurqlIfNotExists renders the DEFINE ANALYZER statement with IF NOT EXISTS
// so it can be re-applied idempotently (e.g. a persistent store applying its
// schema on every connect). Empty tokenizer / filter chains omit their clause
// entirely.
func (a AnalyzerDefinition) ToSurqlIfNotExists() string {
	return a.toSurql(true)
}

func (a AnalyzerDefinition) toSurql(ifNotExists bool) string {
	var b strings.Builder
	b.WriteString("DEFINE ANALYZER")
	if ifNotExists {
		b.WriteString(" IF NOT EXISTS")
	}
	b.WriteString(" ")
	b.WriteString(a.Name)

	if len(a.Tokenizers) > 0 {
		toks := make([]string, len(a.Tokenizers))
		for i, t := range a.Tokenizers {
			toks[i] = string(t)
		}
		b.WriteString(" TOKENIZERS ")
		b.WriteString(strings.Join(toks, ","))
	}

	if len(a.Filters) > 0 {
		filters := make([]string, len(a.Filters))
		for i, f := range a.Filters {
			filters[i] = f.ToSurql()
		}
		b.WriteString(" FILTERS ")
		b.WriteString(strings.Join(filters, ","))
	}

	b.WriteString(";")
	return b.String()
}
