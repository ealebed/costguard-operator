package controller

import (
	"context"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

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

func scaleStatefulSetsToZero(ctx context.Context, c client.Client, namespace, reason string) (int, error) {
	logger := log.FromContext(ctx)
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

		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		if replicas == 0 {
			continue
		}

		patch := client.MergeFrom(sts.DeepCopy())
		if sts.Annotations == nil {
			sts.Annotations = map[string]string{}
		}
		if _, ok := sts.Annotations[preScaleReplicasAnnotation]; !ok {
			sts.Annotations[preScaleReplicasAnnotation] = strconv.FormatInt(int64(replicas), 10)
		}
		zero := int32(0)
		sts.Spec.Replicas = &zero

		if err := c.Patch(ctx, sts, patch); err != nil {
			return n, fmt.Errorf("scale StatefulSet %q/%s to zero: %w", namespace, sts.Name, err)
		}
		n++
		logger.Info("scaled StatefulSet to zero", "reason", reason, "namespace", namespace, "statefulset", sts.Name)
	}

	return n, nil
}

func scaleWorkloadsToZero(ctx context.Context, c client.Client, namespace, reason string) (int, error) {
	n1, err := scaleDeploymentsToZero(ctx, c, namespace, reason)
	if err != nil {
		return n1, err
	}
	n2, err := scaleStatefulSetsToZero(ctx, c, namespace, reason)
	return n1 + n2, err
}
