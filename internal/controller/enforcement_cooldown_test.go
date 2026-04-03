package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCanRestoreAfterScaleDown(t *testing.T) {
	t.Parallel()

	cooldown := time.Minute
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	lastScale := metav1.NewTime(now.Add(-30 * time.Second))

	tests := []struct {
		name   string
		lastOp string
		lastAt *metav1.Time
		want   bool
	}{
		{name: "no prior op", lastOp: "", lastAt: nil, want: true},
		{name: "after restore", lastOp: enforcementOpRestore, lastAt: &lastScale, want: true},
		{name: "scale recent", lastOp: enforcementOpScaleToZero, lastAt: &lastScale, want: false},
		{name: "scale old enough", lastOp: enforcementOpScaleToZero, lastAt: ptrMeta(now.Add(-2 * time.Minute)), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := canRestoreAfterScaleDown(tt.lastOp, tt.lastAt, cooldown, now); got != tt.want {
				t.Fatalf("canRestoreAfterScaleDown() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptrMeta(t time.Time) *metav1.Time {
	mt := metav1.NewTime(t)
	return &mt
}
