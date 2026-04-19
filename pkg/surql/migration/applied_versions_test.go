package migration

import (
	"context"
	"errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestGetAppliedVersions_NilClient(t *testing.T) {
	t.Parallel()

	_, err := GetAppliedVersions(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("err = %v; want ErrValidation", err)
	}
}
