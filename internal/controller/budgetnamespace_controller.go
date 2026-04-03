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
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"

	finopsv1alpha1 "github.com/ealebed/costguard-operator/api/v1alpha1"
)

// BudgetNamespaceReconciler reconciles a BudgetNamespace object
type BudgetNamespaceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

const (
	resourceQuotaName = "costguard-quota"
	limitRangeName    = "costguard-limitrange"

	// costguardExemptLabel skips scale-to-zero for a Deployment's pod template.
	costguardExemptLabel      = "ealebed.github.io/exempt"
	costguardExemptLabelValue = "true"
	// preScaleReplicasAnnotation stores the replica count before scale-to-zero (string int32).
	preScaleReplicasAnnotation = "finops.ealebed.github.io/pre-scale-replicas"
)

// ttlDeleteGracePeriod is the delay between TTL expiry and deleting the managed Namespace.
// In v1alpha1 we keep this fixed to avoid schema churn; later we can make it configurable.
const ttlDeleteGracePeriod = 10 * time.Second

// enforcementPollInterval bounds how long we wait before re-reading ResourceQuota status when needed.
// ResourceQuota is also watched for quicker reconciliation after usage changes.
const enforcementPollInterval = 30 * time.Second

// +kubebuilder:rbac:groups=finops.ealebed.github.io,resources=budgetnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=finops.ealebed.github.io,resources=budgetnamespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=finops.ealebed.github.io,resources=budgetnamespaces/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// Events: core v1 (legacy) and events.k8s.io/v1 (used by mgr.GetEventRecorder).
// +kubebuilder:rbac:groups="";events.k8s.io,resources=events,verbs=create;patch;update

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

	lastEnfAt := budgetNamespace.Status.LastEnforcementAt
	lastEnfOp := budgetNamespace.Status.LastEnforcementOperation
	var recoveryDeferred bool

	now := time.Now()

	var (
		expired          bool
		deleting         bool
		expiresAt        *metav1.Time
		deleteAfterTTL   time.Time
		requeueAfterTTL  time.Duration
		quotaEvaluated   bool
		quotaAtHardLimit bool
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

		if shouldScaleDeploymentsToZero(&budgetNamespace.Spec) {
			cooldown, err := enforcementCooldown(&budgetNamespace.Spec)
			if err != nil {
				r.recordWarning(&budgetNamespace, "InvalidEnforcementCooldown", err.Error())
				return ctrl.Result{}, err
			}

			rq := &corev1.ResourceQuota{}
			rqKey := client.ObjectKey{Namespace: budgetNamespace.Spec.NamespaceName, Name: resourceQuotaName}
			if err := r.Get(ctx, rqKey, rq); err != nil {
				return ctrl.Result{}, fmt.Errorf("get ResourceQuota %q for usage check: %w", resourceQuotaName, err)
			}
			quotaEvaluated = true
			quotaAtHardLimit = quotaUsedAtOrOverHard(rq)

			scaledCount := 0
			if quotaAtHardLimit {
				scaledCount, err = scaleDeploymentsToZero(ctx, r.Client, budgetNamespace.Spec.NamespaceName, "resource-quota")
				if err != nil {
					r.recordWarning(&budgetNamespace, "ScaleToZeroFailed", err.Error())
					return ctrl.Result{}, err
				}
				if scaledCount > 0 {
					t := metav1.Now()
					lastEnfAt = &t
					lastEnfOp = enforcementOpScaleToZero
					r.recordNormal(&budgetNamespace, "ScaledToZero", fmt.Sprintf("Scaled %d Deployment(s) to zero (resource quota)", scaledCount))
				}
			} else if restoreOnRecoveryEnabled(&budgetNamespace.Spec) {
				pending, err := deploymentsPendingRestore(ctx, r.Client, budgetNamespace.Spec.NamespaceName)
				if err != nil {
					return ctrl.Result{}, err
				}
				if pending {
					if canRestoreAfterScaleDown(lastEnfOp, lastEnfAt, cooldown, now) {
						restoredCount, err := restoreDeploymentsFromAnnotation(ctx, r.Client, budgetNamespace.Spec.NamespaceName)
						if err != nil {
							r.recordWarning(&budgetNamespace, "RestoreFailed", err.Error())
							return ctrl.Result{}, err
						}
						if restoredCount > 0 {
							t := metav1.Now()
							lastEnfAt = &t
							lastEnfOp = enforcementOpRestore
							r.recordNormal(&budgetNamespace, "RestoredReplicas",
								fmt.Sprintf("Restored %d Deployment(s) from saved replica counts", restoredCount))
						}
					} else {
						recoveryDeferred = true
						remaining := max(lastEnfAt.Add(cooldown).Sub(now), time.Duration(0))
						msg := fmt.Sprintf("Restore deferred for %s (enforcement cooldown after scale-to-zero)", remaining.Truncate(time.Second))
						r.recordNormal(&budgetNamespace, "RecoveryDeferred", msg)
					}
				}
			}
		}
	} else if expired && !deleting && shouldScaleDeploymentsToZero(&budgetNamespace.Spec) {
		n, err := scaleDeploymentsToZero(ctx, r.Client, budgetNamespace.Spec.NamespaceName, "ttl-grace")
		if err != nil {
			r.recordWarning(&budgetNamespace, "ScaleToZeroFailed", err.Error())
			return ctrl.Result{}, err
		}
		if n > 0 {
			t := metav1.Now()
			lastEnfAt = &t
			lastEnfOp = enforcementOpScaleToZero
			r.recordNormal(&budgetNamespace, "ScaledToZero", fmt.Sprintf("Scaled %d Deployment(s) to zero (TTL grace)", n))
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
	budgetNamespace.Status.LastEnforcementAt = lastEnfAt
	budgetNamespace.Status.LastEnforcementOperation = lastEnfOp

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

	switch {
	case !shouldScaleDeploymentsToZero(&budgetNamespace.Spec):
		apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
			Type:               "OverBudget",
			Status:             metav1.ConditionFalse,
			Reason:             "EnforcementDisabled",
			Message:            "Scale-to-zero enforcement is disabled",
			ObservedGeneration: budgetNamespace.Generation,
		})
	case quotaEvaluated:
		if quotaAtHardLimit {
			apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
				Type:               "OverBudget",
				Status:             metav1.ConditionTrue,
				Reason:             "ResourceQuotaAtOrOverHard",
				Message:            "ResourceQuota usage is at or above a hard limit; non-exempt Deployments were scaled to zero",
				ObservedGeneration: budgetNamespace.Generation,
			})
		} else {
			apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
				Type:               "OverBudget",
				Status:             metav1.ConditionFalse,
				Reason:             "WithinResourceQuota",
				Message:            "ResourceQuota usage is below hard limits",
				ObservedGeneration: budgetNamespace.Generation,
			})
		}
	case budgetNamespace.Spec.TTL != "" && expired:
		apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
			Type:               "OverBudget",
			Status:             metav1.ConditionFalse,
			Reason:             "TTLExpired",
			Message:            "Resource quota usage is not re-evaluated after TTL expiry",
			ObservedGeneration: budgetNamespace.Generation,
		})
	}

	switch {
	case !shouldScaleDeploymentsToZero(&budgetNamespace.Spec) || !quotaEvaluated:
		apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
			Type:               "EnforcementRecoveryDeferred",
			Status:             metav1.ConditionFalse,
			Reason:             "NotApplicable",
			Message:            "Enforcement recovery deferral is not tracked in this state",
			ObservedGeneration: budgetNamespace.Generation,
		})
	case recoveryDeferred:
		apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
			Type:               "EnforcementRecoveryDeferred",
			Status:             metav1.ConditionTrue,
			Reason:             "AwaitingEnforcementCooldown",
			Message:            "Restore is waiting for enforcement cooldown after scale-to-zero",
			ObservedGeneration: budgetNamespace.Generation,
		})
	default:
		apimeta.SetStatusCondition(&budgetNamespace.Status.Conditions, metav1.Condition{
			Type:               "EnforcementRecoveryDeferred",
			Status:             metav1.ConditionFalse,
			Reason:             "NotDeferred",
			Message:            "No restore action is blocked by enforcement cooldown",
			ObservedGeneration: budgetNamespace.Generation,
		})
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

	requeueAfter := requeueAfterTTL
	if shouldScaleDeploymentsToZero(&budgetNamespace.Spec) && !expired {
		if requeueAfter == 0 || enforcementPollInterval < requeueAfter {
			requeueAfter = enforcementPollInterval
		}
	}

	if requeueAfter > 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return ctrl.Result{}, nil
}

func quotaUsedAtOrOverHard(rq *corev1.ResourceQuota) bool {
	for name, hardQty := range rq.Spec.Hard {
		usedQty, ok := rq.Status.Used[name]
		if !ok {
			continue
		}
		if usedQty.Cmp(hardQty) >= 0 {
			return true
		}
	}
	return false
}

func shouldScaleDeploymentsToZero(spec *finopsv1alpha1.BudgetNamespaceSpec) bool {
	enf := spec.Enforcement
	if enf.Action == "None" {
		return false
	}
	if !enf.Enabled {
		return false
	}
	return enf.Action == "" || enf.Action == "ScaleToZero"
}

func scaleDeploymentsToZero(ctx context.Context, c client.Client, namespace, reason string) (int, error) {
	logger := log.FromContext(ctx)
	var list appsv1.DeploymentList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return 0, fmt.Errorf("list Deployments in namespace %q: %w", namespace, err)
	}

	n := 0
	for i := range list.Items {
		deploy := &list.Items[i]

		if deploy.Spec.Template.Labels != nil && deploy.Spec.Template.Labels[costguardExemptLabel] == costguardExemptLabelValue {
			continue
		}

		replicas := int32(1)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}
		if replicas == 0 {
			continue
		}

		patch := client.MergeFrom(deploy.DeepCopy())
		if deploy.Annotations == nil {
			deploy.Annotations = map[string]string{}
		}
		if _, ok := deploy.Annotations[preScaleReplicasAnnotation]; !ok {
			deploy.Annotations[preScaleReplicasAnnotation] = strconv.FormatInt(int64(replicas), 10)
		}
		zero := int32(0)
		deploy.Spec.Replicas = &zero

		if err := c.Patch(ctx, deploy, patch); err != nil {
			return n, fmt.Errorf("scale Deployment %q/%s to zero: %w", namespace, deploy.Name, err)
		}
		n++
		logger.Info("scaled Deployment to zero", "reason", reason, "namespace", namespace, "deployment", deploy.Name)
	}

	return n, nil
}

func (r *BudgetNamespaceReconciler) recordNormal(bn *finopsv1alpha1.BudgetNamespace, reason, message string) {
	if r.Recorder != nil {
		// events.k8s.io/v1 Event API: action mirrors reason when no finer-grained verb exists.
		r.Recorder.Eventf(bn, nil, corev1.EventTypeNormal, reason, reason, "%s", message)
	}
}

func (r *BudgetNamespaceReconciler) recordWarning(bn *finopsv1alpha1.BudgetNamespace, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Eventf(bn, nil, corev1.EventTypeWarning, reason, reason, "%s", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *BudgetNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&finopsv1alpha1.BudgetNamespace{}).
		Watches(
			&corev1.ResourceQuota{},
			handler.EnqueueRequestsFromMapFunc(r.resourceQuotaToRequests),
			builder.WithPredicates(ctrlpredicate.NewPredicateFuncs(func(o client.Object) bool {
				rq, ok := o.(*corev1.ResourceQuota)
				return ok && rq.Name == resourceQuotaName
			})),
		).
		Named("budgetnamespace").
		Complete(r)
}
