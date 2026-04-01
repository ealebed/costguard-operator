/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	finopsv1alpha1 "github.com/ealebed/costguard-operator/api/v1alpha1"
)

// BudgetNamespaceReconciler reconciles a BudgetNamespace object
type BudgetNamespaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=finops.ealebed.github.io,resources=budgetnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=finops.ealebed.github.io,resources=budgetnamespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=finops.ealebed.github.io,resources=budgetnamespaces/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BudgetNamespace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *BudgetNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var budgetNamespace finopsv1alpha1.BudgetNamespace
	if err := r.Get(ctx, req.NamespacedName, &budgetNamespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: budgetNamespace.Spec.NamespaceName,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.Client, namespace, func() error {
		return nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure namespace %q: %w", budgetNamespace.Spec.NamespaceName, err)
	}

	budgetNamespace.Status.ObservedGeneration = budgetNamespace.Generation
	budgetNamespace.Status.ManagedNamespace = budgetNamespace.Spec.NamespaceName
	apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "NamespaceReady",
		Message:            fmt.Sprintf("Namespace %q is present", budgetNamespace.Spec.NamespaceName),
		ObservedGeneration: budgetNamespace.Generation,
	})

	if err := r.Status().Update(ctx, &budgetNamespace); err != nil {
		if apierrors.IsConflict(err) {
			logger.V(1).Info("status update conflict, requeueing", "name", req.NamespacedName)
			return ctrl.Result{Requeue: true}, nil
		}

		return ctrl.Result{}, fmt.Errorf("update BudgetNamespace status: %w", err)
	}

	logger.Info("reconciled BudgetNamespace", "budgetNamespace", req.NamespacedName, "managedNamespace", budgetNamespace.Spec.NamespaceName)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BudgetNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&finopsv1alpha1.BudgetNamespace{}).
		Named("budgetnamespace").
		Complete(r)
}
