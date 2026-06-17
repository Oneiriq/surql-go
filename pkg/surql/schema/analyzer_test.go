package schema

import (
	stdErrors "errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestTokenizer_Strings(t *testing.T) {
	cases := map[Tokenizer]string{
		TokenizerBlank: "blank",
		TokenizerCamel: "camel",
		TokenizerClass: "class",
		TokenizerPunct: "punct",
	}
	for tok, want := range cases {
		if got := tok.String(); got != want {
			t.Errorf("Tokenizer(%v).String() = %q, want %q", tok, got, want)
		}
	}
}

func TestTokenFilter_Renders(t *testing.T) {
	cases := []struct {
		filter TokenFilter
		want   string
	}{
		{ASCII(), "ascii"},
		{Lowercase(), "lowercase"},
		{Uppercase(), "uppercase"},
		{EdgeNgram(2, 10), "edgengram(2,10)"},
		{Ngram(1, 3), "ngram(1,3)"},
		{Snowball("english"), "snowball(english)"},
	}
	for _, c := range cases {
		if got := c.filter.ToSurql(); got != c.want {
			t.Errorf("filter.ToSurql() = %q, want %q", got, c.want)
		}
	}
}

func TestAnalyzer_MinimalRendersNameOnly(t *testing.T) {
	got := Analyzer("plain").ToSurql()
	if want := "DEFINE ANALYZER plain;"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAnalyzer_RendersTokenizersAndFilters(t *testing.T) {
	a := Analyzer("text_en",
		WithTokenizers(TokenizerClass, TokenizerCamel),
		WithFilters(Lowercase(), ASCII()),
	)
	got := a.ToSurql()
	want := "DEFINE ANALYZER text_en TOKENIZERS class,camel FILTERS lowercase,ascii;"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAnalyzer_IfNotExists(t *testing.T) {
	got := StandardAnalyzer("std").ToSurqlIfNotExists()
	want := "DEFINE ANALYZER IF NOT EXISTS std TOKENIZERS class FILTERS lowercase,ascii;"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestStandardAnalyzer_IsClassLowercaseAscii(t *testing.T) {
	got := StandardAnalyzer("std").ToSurql()
	want := "DEFINE ANALYZER std TOKENIZERS class FILTERS lowercase,ascii;"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAnalyzer_WithSnowball(t *testing.T) {
	a := StandardAnalyzer("text_en", WithFilter(Snowball("english")))
	got := a.ToSurql()
	want := "DEFINE ANALYZER text_en TOKENIZERS class FILTERS lowercase,ascii,snowball(english);"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAnalyzer_ValidateRejectsEmptyName(t *testing.T) {
	a := Analyzer("")
	err := a.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty name")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestAnalyzer_ValidatePassesWithName(t *testing.T) {
	if err := StandardAnalyzer("text_en").Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
