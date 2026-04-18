package migration

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMigrationState_IsValid(t *testing.T) {
	cases := []struct {
		in   MigrationState
		want bool
	}{
		{MigrationStatePending, true},
		{MigrationStateApplied, true},
		{MigrationStateFailed, true},
		{MigrationState(""), false},
		{MigrationState("bogus"), false},
	}
	for _, tc := range cases {
		if got := tc.in.IsValid(); got != tc.want {
			t.Errorf("MigrationState(%q).IsValid() = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestMigrationState_String(t *testing.T) {
	pairs := map[MigrationState]string{
		MigrationStatePending: "pending",
		MigrationStateApplied: "applied",
		MigrationStateFailed:  "failed",
	}
	for state, want := range pairs {
		if got := state.String(); got != want {
			t.Errorf("MigrationState(%q).String() = %q, want %q", state, got, want)
		}
	}
}

func TestMigrationDirection_IsValid(t *testing.T) {
	cases := []struct {
		in   MigrationDirection
		want bool
	}{
		{MigrationDirectionUp, true},
		{MigrationDirectionDown, true},
		{MigrationDirection(""), false},
		{MigrationDirection("sideways"), false},
	}
	for _, tc := range cases {
		if got := tc.in.IsValid(); got != tc.want {
			t.Errorf("MigrationDirection(%q).IsValid() = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestMigrationDirection_String(t *testing.T) {
	if MigrationDirectionUp.String() != "up" {
		t.Errorf("up direction string = %q", MigrationDirectionUp.String())
	}
	if MigrationDirectionDown.String() != "down" {
		t.Errorf("down direction string = %q", MigrationDirectionDown.String())
	}
}

func TestDiffOperation_IsValid(t *testing.T) {
	valid := []DiffOperation{
		DiffOperationAddTable,
		DiffOperationDropTable,
		DiffOperationAddField,
		DiffOperationDropField,
		DiffOperationModifyField,
		DiffOperationAddIndex,
		DiffOperationDropIndex,
		DiffOperationAddEvent,
		DiffOperationDropEvent,
		DiffOperationModifyPermissions,
	}
	for _, op := range valid {
		if !op.IsValid() {
			t.Errorf("DiffOperation(%q) should be valid", op)
		}
	}
	invalid := []DiffOperation{"", "unknown", "ADD_TABLE"}
	for _, op := range invalid {
		if op.IsValid() {
			t.Errorf("DiffOperation(%q) should be invalid", op)
		}
	}
}

func TestDiffOperation_String(t *testing.T) {
	if DiffOperationAddTable.String() != "add_table" {
		t.Errorf("add_table string = %q", DiffOperationAddTable.String())
	}
}

func TestMigrationPlan_CountAndIsEmpty(t *testing.T) {
	empty := MigrationPlan{Direction: MigrationDirectionUp}
	if empty.Count() != 0 {
		t.Errorf("empty plan Count = %d", empty.Count())
	}
	if !empty.IsEmpty() {
		t.Errorf("empty plan IsEmpty = false")
	}

	full := MigrationPlan{
		Migrations: []Migration{
			{Version: "20260101_000000", Description: "a"},
			{Version: "20260102_000000", Description: "b"},
		},
		Direction: MigrationDirectionDown,
	}
	if full.Count() != 2 {
		t.Errorf("full plan Count = %d", full.Count())
	}
	if full.IsEmpty() {
		t.Errorf("full plan IsEmpty = true")
	}
}

func TestMigration_JSONRoundTrip(t *testing.T) {
	m := Migration{
		Version:        "20260102_120000",
		Description:    "create user table",
		Path:           "migrations/20260102_120000_create_user_table.surql",
		UpStatements:   []string{"DEFINE TABLE user SCHEMAFULL;"},
		DownStatements: []string{"REMOVE TABLE user;"},
		Checksum:       "abc123",
		DependsOn:      []string{"20260101_000000_init"},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round Migration
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.Version != m.Version || round.Description != m.Description {
		t.Errorf("mismatch after round trip: %+v vs %+v", round, m)
	}
	if len(round.UpStatements) != 1 || round.UpStatements[0] != m.UpStatements[0] {
		t.Errorf("up statements mismatch: %+v", round.UpStatements)
	}
	if len(round.DownStatements) != 1 || round.DownStatements[0] != m.DownStatements[0] {
		t.Errorf("down statements mismatch: %+v", round.DownStatements)
	}
	if round.Checksum != m.Checksum {
		t.Errorf("checksum mismatch: %q", round.Checksum)
	}
	if len(round.DependsOn) != 1 || round.DependsOn[0] != m.DependsOn[0] {
		t.Errorf("depends_on mismatch: %+v", round.DependsOn)
	}
}

func TestMigration_JSONOmitsEmptyOptionalFields(t *testing.T) {
	m := Migration{
		Version:        "20260102_120000",
		Description:    "noop",
		UpStatements:   []string{"SELECT 1;"},
		DownStatements: []string{"SELECT 1;"},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, key := range []string{`"path":`, `"checksum":`, `"depends_on":`} {
		if contains(s, key) {
			t.Errorf("expected %s to be omitted, got %s", key, s)
		}
	}
}

func TestMigrationHistory_JSONRoundTrip(t *testing.T) {
	execMs := int64(1234)
	applied := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	h := MigrationHistory{
		Version:         "20260102_120000",
		Description:     "create user",
		AppliedAt:       applied,
		Checksum:        "deadbeef",
		ExecutionTimeMs: &execMs,
	}
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round MigrationHistory
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !round.AppliedAt.Equal(applied) {
		t.Errorf("applied_at mismatch: %v", round.AppliedAt)
	}
	if round.ExecutionTimeMs == nil || *round.ExecutionTimeMs != execMs {
		t.Errorf("execution_time_ms mismatch: %v", round.ExecutionTimeMs)
	}
}

func TestMigrationHistory_OmitsExecutionTimeWhenNil(t *testing.T) {
	h := MigrationHistory{
		Version:     "20260102_120000",
		Description: "x",
		AppliedAt:   time.Unix(0, 0).UTC(),
		Checksum:    "x",
	}
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(string(data), `"execution_time_ms":`) {
		t.Errorf("execution_time_ms should be omitted: %s", data)
	}
}

func TestMigrationPlan_JSONRoundTrip(t *testing.T) {
	p := MigrationPlan{
		Migrations: []Migration{
			{
				Version:        "20260102_120000",
				Description:    "x",
				UpStatements:   []string{"SELECT 1;"},
				DownStatements: []string{"SELECT 0;"},
			},
		},
		Direction: MigrationDirectionUp,
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round MigrationPlan
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.Direction != MigrationDirectionUp {
		t.Errorf("direction mismatch: %v", round.Direction)
	}
	if len(round.Migrations) != 1 {
		t.Errorf("migrations len = %d", len(round.Migrations))
	}
}

func TestMigrationMetadata_JSONRoundTrip(t *testing.T) {
	md := MigrationMetadata{
		Version:     "20260102_120000",
		Description: "test",
		Author:      "surql",
		DependsOn:   []string{"20260101_000000_init"},
	}
	data, err := json.Marshal(md)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round MigrationMetadata
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.Version != md.Version || round.Description != md.Description || round.Author != md.Author {
		t.Errorf("scalars mismatch: %+v vs %+v", round, md)
	}
	if len(round.DependsOn) != 1 || round.DependsOn[0] != md.DependsOn[0] {
		t.Errorf("depends_on mismatch: %+v", round.DependsOn)
	}
}

func TestMigrationStatus_JSONRoundTrip(t *testing.T) {
	applied := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	s := MigrationStatus{
		Migration: Migration{
			Version:        "20260102_120000",
			Description:    "x",
			UpStatements:   []string{"SELECT 1;"},
			DownStatements: []string{"SELECT 0;"},
		},
		State:     MigrationStateApplied,
		AppliedAt: &applied,
		Error:     "",
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round MigrationStatus
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.State != MigrationStateApplied {
		t.Errorf("state mismatch: %v", round.State)
	}
	if round.AppliedAt == nil || !round.AppliedAt.Equal(applied) {
		t.Errorf("applied_at mismatch: %v", round.AppliedAt)
	}
}

func TestSchemaDiff_JSONRoundTrip(t *testing.T) {
	d := SchemaDiff{
		Operation:   DiffOperationAddTable,
		Table:       "user",
		Description: "add user table",
		ForwardSQL:  "DEFINE TABLE user SCHEMAFULL;",
		BackwardSQL: "REMOVE TABLE user;",
		Details:     map[string]any{"mode": "schemafull"},
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round SchemaDiff
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.Operation != DiffOperationAddTable {
		t.Errorf("operation mismatch: %v", round.Operation)
	}
	if round.Table != "user" || round.ForwardSQL != "DEFINE TABLE user SCHEMAFULL;" {
		t.Errorf("scalars mismatch: %+v", round)
	}
	if got := round.Details["mode"]; got != "schemafull" {
		t.Errorf("details[mode] = %v", got)
	}
}

func TestSchemaDiff_OmitsEmptyOptionalFields(t *testing.T) {
	d := SchemaDiff{
		Operation:   DiffOperationAddTable,
		Table:       "user",
		Description: "x",
		ForwardSQL:  "SELECT 1;",
		BackwardSQL: "SELECT 0;",
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, key := range []string{`"field":`, `"index":`, `"event":`, `"details":`} {
		if contains(s, key) {
			t.Errorf("expected %s omitted, got %s", key, s)
		}
	}
}

// contains is a tiny substring helper to avoid pulling in strings in tests
// that only need this one check.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
