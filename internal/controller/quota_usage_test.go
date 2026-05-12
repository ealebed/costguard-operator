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
					Hard: corev1.ResourceList{corev1.ResourcePods: hPods},
				},
			},
			want: false,
		},
		{
			name: "used below hard",
			rq: &corev1.ResourceQuota{
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{corev1.ResourcePods: hPods},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("1")},
				},
			},
			want: false,
		},
		{
			name: "used equals hard",
			rq: &corev1.ResourceQuota{
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{corev1.ResourcePods: hPods},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("2")},
				},
			},
			want: true,
		},
		{
			name: "used above hard",
			rq: &corev1.ResourceQuota{
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{corev1.ResourcePods: hPods},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("3")},
				},
			},
			want: true,
		},
		{
			name: "one dimension at limit among several",
			rq: &corev1.ResourceQuota{
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						corev1.ResourcePods:           hPods,
						corev1.ResourceRequestsCPU:    hCPU,
						corev1.ResourceRequestsMemory: resource.MustParse("1Gi"),
					},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: corev1.ResourceList{
						corev1.ResourcePods:           resource.MustParse("1"),
						corev1.ResourceRequestsCPU:    hCPU,
						corev1.ResourceRequestsMemory: resource.MustParse("100Mi"),
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
