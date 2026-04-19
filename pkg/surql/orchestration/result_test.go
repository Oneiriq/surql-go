package orchestration

import (
	"testing"
	"time"
)

func TestDeploymentStatus_IsValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   DeploymentStatus
		want bool
	}{
		{DeploymentStatusPending, true},
		{DeploymentStatusInProgress, true},
		{DeploymentStatusSuccess, true},
		{DeploymentStatusFailed, true},
		{DeploymentStatusRolledBack, true},
		{DeploymentStatus(""), false},
		{DeploymentStatus("bogus"), false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.in), func(t *testing.T) {
			t.Parallel()
			if got := tc.in.IsValid(); got != tc.want {
				t.Fatalf("IsValid(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestDeploymentStatus_String(t *testing.T) {
	t.Parallel()

	if DeploymentStatusSuccess.String() != "success" {
		t.Fatalf("String = %q, want success", DeploymentStatusSuccess.String())
	}
}

func TestDeploymentResult_DurationSeconds(t *testing.T) {
	t.Parallel()

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		input DeploymentResult
		want  float64
	}{
		{
			name: "completed 2s",
			input: DeploymentResult{
				StartedAt:   start,
				CompletedAt: start.Add(2 * time.Second),
			},
			want: 2.0,
		},
		{
			name:  "in progress returns 0",
			input: DeploymentResult{StartedAt: start},
			want:  0,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.input.DurationSeconds(); got != tc.want {
				t.Fatalf("DurationSeconds = %v, want %v", got, tc.want)
			}
		})
	}
}
