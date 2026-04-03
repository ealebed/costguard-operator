package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	finopsv1alpha1 "github.com/ealebed/costguard-operator/api/v1alpha1"
)

func (r *BudgetNamespaceReconciler) resourceQuotaToRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	rq, ok := obj.(*corev1.ResourceQuota)
	if !ok || rq.Name != resourceQuotaName {
		return nil
	}

	var list finopsv1alpha1.BudgetNamespaceList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}

	var out []reconcile.Request
	for i := range list.Items {
		if list.Items[i].Spec.NamespaceName != rq.Namespace {
			continue
		}
		out = append(out, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: list.Items[i].Namespace,
				Name:      list.Items[i].Name,
			},
		})
	}
	return out
}
