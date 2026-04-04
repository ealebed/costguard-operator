package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	finopsv1alpha1 "github.com/ealebed/costguard-operator/api/v1alpha1"
)

const (
	enforcementOpScaleToZero = "ScaleToZero"
	enforcementOpRestore     = "Restore"
)

const defaultEnforcementCooldown = 2 * time.Minute

func enforcementCooldown(spec *finopsv1alpha1.BudgetNamespaceSpec) (time.Duration, error) {
	s := spec.Enforcement.EnforcementCooldown
	if s == "" {
		return defaultEnforcementCooldown, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("parse spec.enforcement.enforcementCooldown %q: %w", s, err)
	}
	return d, nil
}

func restoreOnRecoveryEnabled(spec *finopsv1alpha1.BudgetNamespaceSpec) bool {
	return spec.Enforcement.RestoreOnRecovery
}

// canRestoreAfterScaleDown gates restore so we do not immediately bring workloads back
// right after a scale-to-zero (anti-flap).
func canRestoreAfterScaleDown(lastOp string, lastAt *metav1.Time, cooldown time.Duration, now time.Time) bool {
	if lastOp != enforcementOpScaleToZero {
		return true
	}
	if lastAt == nil {
		return true
	}
	return !now.Before(lastAt.Add(cooldown))
}

func deploymentsPendingRestore(ctx context.Context, c client.Client, namespace string) (bool, error) {
	var list appsv1.DeploymentList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return false, fmt.Errorf("list Deployments in namespace %q: %w", namespace, err)
	}

	for i := range list.Items {
		deploy := &list.Items[i]
		if deploy.Spec.Template.Labels != nil && deploy.Spec.Template.Labels[costguardExemptLabel] == costguardExemptLabelValue {
			continue
		}
		replicas := int32(0)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}
		if replicas != 0 {
			continue
		}
		if deploy.Annotations == nil {
			continue
		}
		if _, ok := deploy.Annotations[preScaleReplicasAnnotation]; ok {
			return true, nil
		}
	}
	return false, nil
}

func statefulSetsPendingRestore(ctx context.Context, c client.Client, namespace string) (bool, error) {
	var list appsv1.StatefulSetList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return false, fmt.Errorf("list StatefulSets in namespace %q: %w", namespace, err)
	}

	for i := range list.Items {
		sts := &list.Items[i]
		if sts.Spec.Template.Labels != nil && sts.Spec.Template.Labels[costguardExemptLabel] == costguardExemptLabelValue {
			continue
		}
		replicas := int32(0)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		if replicas != 0 {
			continue
		}
		if sts.Annotations == nil {
			continue
		}
		if _, ok := sts.Annotations[preScaleReplicasAnnotation]; ok {
			return true, nil
		}
	}
	return false, nil
}

func workloadsPendingRestore(ctx context.Context, c client.Client, namespace string) (bool, error) {
	d, err := deploymentsPendingRestore(ctx, c, namespace)
	if err != nil || d {
		return d, err
	}
	return statefulSetsPendingRestore(ctx, c, namespace)
}

func restoreDeploymentsFromAnnotation(ctx context.Context, c client.Client, namespace string) (int, error) {
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

		replicas := int32(0)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}
		if replicas != 0 {
			continue
		}

		raw, ok := deploy.Annotations[preScaleReplicasAnnotation]
		if !ok {
			continue
		}

		prior, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || prior <= 0 {
			continue
		}

		patch := client.MergeFrom(deploy.DeepCopy())
		target := int32(prior)
		deploy.Spec.Replicas = &target
		delete(deploy.Annotations, preScaleReplicasAnnotation)

		if err := c.Patch(ctx, deploy, patch); err != nil {
			return n, fmt.Errorf("restore Deployment %q/%s: %w", namespace, deploy.Name, err)
		}
		n++
	}

	return n, nil
}

func restoreStatefulSetsFromAnnotation(ctx context.Context, c client.Client, namespace string) (int, error) {
	var list appsv1.StatefulSetList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return 0, fmt.Errorf("list StatefulSets in namespace %q: %w", namespace, err)
	}

	n := 0
	for i := range list.Items {
		sts := &list.Items[i]

		if sts.Spec.Template.Labels != nil && sts.Spec.Template.Labels[costguardExemptLabel] == costguardExemptLabelValue {
			continue
		}

		replicas := int32(0)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		if replicas != 0 {
			continue
		}

		raw, ok := sts.Annotations[preScaleReplicasAnnotation]
		if !ok {
			continue
		}

		prior, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || prior <= 0 {
			continue
		}

		patch := client.MergeFrom(sts.DeepCopy())
		target := int32(prior)
		sts.Spec.Replicas = &target
		delete(sts.Annotations, preScaleReplicasAnnotation)

		if err := c.Patch(ctx, sts, patch); err != nil {
			return n, fmt.Errorf("restore StatefulSet %q/%s: %w", namespace, sts.Name, err)
		}
		n++
	}

	return n, nil
}

func restoreScaledWorkloads(ctx context.Context, c client.Client, namespace string) (int, error) {
	n1, err := restoreDeploymentsFromAnnotation(ctx, c, namespace)
	if err != nil {
		return n1, err
	}
	n2, err := restoreStatefulSetsFromAnnotation(ctx, c, namespace)
	return n1 + n2, err
}
