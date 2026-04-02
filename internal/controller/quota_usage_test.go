package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestQuotaUsedAtOrOverHard(t *testing.T) {
	t.Parallel()

	hPods := resource.MustParse("2")
	hCPU := resource.MustParse("1")

	tests := []struct {
		name string
		rq   *corev1.ResourceQuota
		want bool
	}{
		{
			name: "empty status",
			rq: &corev1.ResourceQuota{
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{"pods": hPods},
				},
			},
			want: false,
		},
		{
			name: "used below hard",
			rq: &corev1.ResourceQuota{
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{"pods": hPods},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: corev1.ResourceList{"pods": resource.MustParse("1")},
				},
			},
			want: false,
		},
		{
			name: "used equals hard",
			rq: &corev1.ResourceQuota{
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{"pods": hPods},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: corev1.ResourceList{"pods": resource.MustParse("2")},
				},
			},
			want: true,
		},
		{
			name: "used above hard",
			rq: &corev1.ResourceQuota{
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{"pods": hPods},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: corev1.ResourceList{"pods": resource.MustParse("3")},
				},
			},
			want: true,
		},
		{
			name: "one dimension at limit among several",
			rq: &corev1.ResourceQuota{
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						"pods":            hPods,
						"requests.cpu":    hCPU,
						"requests.memory": resource.MustParse("1Gi"),
					},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: corev1.ResourceList{
						"pods":            resource.MustParse("1"),
						"requests.cpu":    hCPU,
						"requests.memory": resource.MustParse("100Mi"),
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := quotaUsedAtOrOverHard(tt.rq); got != tt.want {
				t.Fatalf("quotaUsedAtOrOverHard() = %v, want %v", got, tt.want)
			}
		})
	}
}
