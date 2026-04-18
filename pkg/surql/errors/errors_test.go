package errors

import (
	"errors"
	"io"
	"testing"
)

func TestNew_IncludesReason(t *testing.T) {
	err := New(ErrQuery, "missing table")
	if err.Error() != "query error: missing table" {
		t.Errorf("got %q", err.Error())
	}
}

func TestWrap_FormatsChain(t *testing.T) {
	err := Wrap(ErrConnection, "dialing", io.EOF)
	want := "connection error: dialing: EOF"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestIs_MatchesSentinel(t *testing.T) {
	err := New(ErrValidation, "bad input")
	if !errors.Is(err, ErrValidation) {
		t.Error("expected errors.Is(err, ErrValidation) to be true")
	}
	if errors.Is(err, ErrQuery) {
		t.Error("unexpected match on unrelated sentinel")
	}
}

func TestIs_MatchesWrappedChain(t *testing.T) {
	inner := errors.New("inner")
	err := Wrap(ErrQuery, "outer", inner)
	if !errors.Is(err, inner) {
		t.Error("expected errors.Is to see inner in chain")
	}
	if !errors.Is(err, ErrQuery) {
		t.Error("expected errors.Is to see sentinel kind")
	}
}

func TestAs_ReturnsTypedError(t *testing.T) {
	err := error(Wrap(ErrQuery, "outer", io.EOF))
	var se *SurqlError
	if !errors.As(err, &se) {
		t.Fatal("expected errors.As to extract SurqlError")
	}
	if se.Kind != ErrQuery {
		t.Errorf("expected Kind ErrQuery, got %v", se.Kind)
	}
}

func TestUnwrap_PrefersWrapped(t *testing.T) {
	err := Wrap(ErrQuery, "ctx", io.EOF)
	if errors.Unwrap(err) != io.EOF {
		t.Errorf("expected Unwrap to return io.EOF, got %v", errors.Unwrap(err))
	}
}

func TestUnwrap_FallsBackToKind(t *testing.T) {
	err := New(ErrQuery, "ctx")
	if errors.Unwrap(err) != ErrQuery {
		t.Errorf("expected Unwrap to return ErrQuery, got %v", errors.Unwrap(err))
	}
}

func TestNewf_Formats(t *testing.T) {
	err := Newf(ErrValidation, "bad %s (%d)", "field", 42)
	if err.Error() != "validation error: bad field (42)" {
		t.Errorf("got %q", err.Error())
	}
}

func TestWrapf_Formats(t *testing.T) {
	err := Wrapf(ErrQuery, io.EOF, "decoding %s", "row")
	if err.Error() != "query error: decoding row: EOF" {
		t.Errorf("got %q", err.Error())
	}
}

func TestNilError_HandlesGracefully(t *testing.T) {
	var e *SurqlError
	if e.Error() != "<nil>" {
		t.Errorf("got %q", e.Error())
	}
	if e.Unwrap() != nil {
		t.Error("nil Unwrap should be nil")
	}
	if !e.Is(nil) {
		t.Error("nil should match nil target")
	}
}

func TestError_NoReasonOrWrapped_FallsBackToKind(t *testing.T) {
	e := &SurqlError{Kind: ErrDatabase}
	if e.Error() != "database error" {
		t.Errorf("got %q", e.Error())
	}
}
