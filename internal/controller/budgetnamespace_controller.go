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
	"maps"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	finopsv1alpha1 "github.com/ealebed/costguard-operator/api/v1alpha1"
)

// BudgetNamespaceReconciler reconciles a BudgetNamespace object
type BudgetNamespaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	resourceQuotaName = "costguard-quota"
	limitRangeName    = "costguard-limitrange"
)

// ttlDeleteGracePeriod is the delay between TTL expiry and deleting the managed Namespace.
// In v1alpha1 we keep this fixed to avoid schema churn; later we can make it configurable.
const ttlDeleteGracePeriod = 10 * time.Second

// +kubebuilder:rbac:groups=finops.ealebed.github.io,resources=budgetnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=finops.ealebed.github.io,resources=budgetnamespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=finops.ealebed.github.io,resources=budgetnamespaces/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BudgetNamespace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
//
//nolint:gocyclo
func (r *BudgetNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var budgetNamespace finopsv1alpha1.BudgetNamespace
	if err := r.Get(ctx, req.NamespacedName, &budgetNamespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	now := time.Now()

	var (
		expired         bool
		deleting        bool
		expiresAt       *metav1.Time
		deleteAfterTTL  time.Time
		requeueAfterTTL time.Duration
	)

	if budgetNamespace.Spec.TTL != "" {
		ttlDuration, err := time.ParseDuration(budgetNamespace.Spec.TTL)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("parse spec.ttl %q: %w", budgetNamespace.Spec.TTL, err)
		}

		// TTL is computed from the custom resource creation timestamp.
		// This keeps behavior deterministic and avoids needing extra spec fields.
		expiry := budgetNamespace.CreationTimestamp.Add(ttlDuration)
		expiresAt = &metav1.Time{Time: expiry}
		deleteAfterTTL = expiry.Add(ttlDeleteGracePeriod)

		expired = !now.Before(expiry)
		deleting = expired && !now.Before(deleteAfterTTL)

		var target time.Time
		switch {
		case !expired:
			target = expiry
		case !deleting:
			target = deleteAfterTTL
		default:
			// deletion started (or already due)
		}
		if !target.IsZero() && target.After(now) {
			requeueAfterTTL = min(time.Until(target), 10*time.Minute)
		} else if deleting {
			// After initiating deletion, keep polling briefly until the API reflects it.
			requeueAfterTTL = 10 * time.Second
		}
	}

	// Namespace resources:
	// - If TTL is not expired: ensure Namespace + labels + quota/limitrange.
	// - If TTL is expired: delete Namespace after the grace period.
	if !expired {
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: budgetNamespace.Spec.NamespaceName,
			},
		}

		_, err := controllerutil.CreateOrPatch(ctx, r.Client, namespace, func() error {
			// Merge desired labels/annotations into the (possibly existing) Namespace.
			if budgetNamespace.Spec.Labels != nil {
				if namespace.Labels == nil {
					namespace.Labels = make(map[string]string, len(budgetNamespace.Spec.Labels))
				}
				maps.Copy(namespace.Labels, budgetNamespace.Spec.Labels)
			}

			if budgetNamespace.Spec.Annotations != nil {
				if namespace.Annotations == nil {
					namespace.Annotations = make(map[string]string, len(budgetNamespace.Spec.Annotations))
				}
				maps.Copy(namespace.Annotations, budgetNamespace.Spec.Annotations)
			}
			return nil
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("ensure namespace %q: %w", budgetNamespace.Spec.NamespaceName, err)
		}

		// Apply ResourceQuota and LimitRange so the namespace is budget-safe.
		// Interpret `spec.quota` as request-based hard quotas, because GKE cost allocation attributes
		// costs based on requests.
		requestCPU, err := resource.ParseQuantity(budgetNamespace.Spec.Quota.CPU)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("parse quota.cpu: %w", err)
		}
		requestMemory, err := resource.ParseQuantity(budgetNamespace.Spec.Quota.Memory)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("parse quota.memory: %w", err)
		}
		requestStorage, err := resource.ParseQuantity(budgetNamespace.Spec.Quota.Storage)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("parse quota.storage: %w", err)
		}
		pvcCount := resource.MustParse(fmt.Sprintf("%d", budgetNamespace.Spec.Quota.PersistentVolumeClaims))
		podCount := resource.MustParse(fmt.Sprintf("%d", budgetNamespace.Spec.Quota.Pods))

		resourceQuota := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceQuotaName,
				Namespace: budgetNamespace.Spec.NamespaceName,
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					"requests.cpu":           requestCPU,
					"requests.memory":        requestMemory,
					"requests.storage":       requestStorage,
					"persistentvolumeclaims": pvcCount,
					"pods":                   podCount,
				},
			},
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, resourceQuota, func() error {
			resourceQuota.Spec.Hard = corev1.ResourceList{
				"requests.cpu":           requestCPU,
				"requests.memory":        requestMemory,
				"requests.storage":       requestStorage,
				"persistentvolumeclaims": pvcCount,
				"pods":                   podCount,
			}
			return nil
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("apply ResourceQuota in namespace %q: %w", budgetNamespace.Spec.NamespaceName, err)
		}

		requestCPUDefault, err := resource.ParseQuantity(budgetNamespace.Spec.Defaults.RequestCPU)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("parse defaults.requestCPU: %w", err)
		}
		requestMemoryDefault, err := resource.ParseQuantity(budgetNamespace.Spec.Defaults.RequestMemory)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("parse defaults.requestMemory: %w", err)
		}
		limitCPUDefault, err := resource.ParseQuantity(budgetNamespace.Spec.Defaults.LimitCPU)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("parse defaults.limitCPU: %w", err)
		}
		limitMemoryDefault, err := resource.ParseQuantity(budgetNamespace.Spec.Defaults.LimitMemory)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("parse defaults.limitMemory: %w", err)
		}

		limitRange := &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{
				Name:      limitRangeName,
				Namespace: budgetNamespace.Spec.NamespaceName,
			},
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type: corev1.LimitTypeContainer,
						DefaultRequest: corev1.ResourceList{
							"cpu":    requestCPUDefault,
							"memory": requestMemoryDefault,
						},
						Default: corev1.ResourceList{
							"cpu":    limitCPUDefault,
							"memory": limitMemoryDefault,
						},
					},
				},
			},
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, limitRange, func() error {
			limitRange.Spec.Limits = []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					DefaultRequest: corev1.ResourceList{
						"cpu":    requestCPUDefault,
						"memory": requestMemoryDefault,
					},
					Default: corev1.ResourceList{
						"cpu":    limitCPUDefault,
						"memory": limitMemoryDefault,
					},
				},
			}
			return nil
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("apply LimitRange in namespace %q: %w", budgetNamespace.Spec.NamespaceName, err)
		}
	} else if deleting {
		// TTL expired: delete the Namespace after grace period.
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: budgetNamespace.Spec.NamespaceName,
			},
		}

		if err := r.Delete(ctx, namespace); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("delete expired namespace %q: %w", budgetNamespace.Spec.NamespaceName, err)
		}
	}

	// Status:
	budgetNamespace.Status.ObservedGeneration = budgetNamespace.Generation
	budgetNamespace.Status.ManagedNamespace = budgetNamespace.Spec.NamespaceName
	budgetNamespace.Status.ExpiresAt = expiresAt

	readyStatus := metav1.ConditionTrue
	readyReason := "NamespaceReady"
	readyMessage := fmt.Sprintf("Namespace %q is present", budgetNamespace.Spec.NamespaceName)

	if budgetNamespace.Spec.TTL != "" {
		if expired {
			apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
				Type:               "Expired",
				Status:             metav1.ConditionTrue,
				Reason:             "TTLExpired",
				Message:            fmt.Sprintf("Namespace TTL expired; deleting after %s", ttlDeleteGracePeriod),
				ObservedGeneration: budgetNamespace.Generation,
			})
		} else {
			apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
				Type:               "Expired",
				Status:             metav1.ConditionFalse,
				Reason:             "TTLActive",
				Message:            "TTL is active",
				ObservedGeneration: budgetNamespace.Generation,
			})
		}

		if expired {
			readyStatus = metav1.ConditionFalse
			readyReason = "TTLExpired"
			readyMessage = fmt.Sprintf("Namespace %q is expired by TTL", budgetNamespace.Spec.NamespaceName)
		}
	}

	apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             readyStatus,
		Reason:             readyReason,
		Message:            readyMessage,
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

	if requeueAfterTTL > 0 {
		return ctrl.Result{RequeueAfter: requeueAfterTTL}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BudgetNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&finopsv1alpha1.BudgetNamespace{}).
		Named("budgetnamespace").
		Complete(r)
}
