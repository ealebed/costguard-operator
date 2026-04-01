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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	finopsv1alpha1 "github.com/ealebed/costguard-operator/api/v1alpha1"
)

var _ = Describe("BudgetNamespace Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		budgetnamespace := &finopsv1alpha1.BudgetNamespace{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind BudgetNamespace")
			err := k8sClient.Get(ctx, typeNamespacedName, budgetnamespace)
			if err != nil && errors.IsNotFound(err) {
				resource := &finopsv1alpha1.BudgetNamespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: finopsv1alpha1.BudgetNamespaceSpec{
						NamespaceName: "managed-test-namespace",
						Quota: finopsv1alpha1.BudgetNamespaceQuotaSpec{
							CPU:                    "1",
							Memory:                 "1Gi",
							Storage:                "10Gi",
							PersistentVolumeClaims: 1,
							Pods:                   5,
						},
						Defaults: finopsv1alpha1.BudgetNamespaceDefaultsSpec{
							RequestCPU:    "100m",
							RequestMemory: "128Mi",
							LimitCPU:      "250m",
							LimitMemory:   "256Mi",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &finopsv1alpha1.BudgetNamespace{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance BudgetNamespace")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &BudgetNamespaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			namespace := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "managed-test-namespace"}, namespace)).To(Succeed())

			reconciled := &finopsv1alpha1.BudgetNamespace{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, reconciled)).To(Succeed())
			Expect(reconciled.Status.ManagedNamespace).To(Equal("managed-test-namespace"))
			Expect(reconciled.Status.ObservedGeneration).To(Equal(reconciled.Generation))
			Expect(reconciled.Status.Conditions).NotTo(BeEmpty())
			readyCondition := meta.FindStatusCondition(reconciled.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal("NamespaceReady"))
		})
	})
})
