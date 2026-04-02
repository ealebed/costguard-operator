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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
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
						Labels: map[string]string{
							"ealebed.github.io/team": "platform",
						},
						Annotations: map[string]string{
							"ealebed.github.io/owner": "ealebed",
						},
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
						TTL: "2h",
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

			Expect(namespace.Labels["ealebed.github.io/team"]).To(Equal("platform"))
			Expect(namespace.Annotations["ealebed.github.io/owner"]).To(Equal("ealebed"))

			resourceQuota := &corev1.ResourceQuota{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "costguard-quota",
				Namespace: "managed-test-namespace",
			}, resourceQuota)).To(Succeed())
			expectedCPU := resource.MustParse("1")
			qtyCPU := resourceQuota.Spec.Hard["requests.cpu"]
			Expect((&qtyCPU).Cmp(expectedCPU)).To(Equal(0))

			expectedMemory := resource.MustParse("1Gi")
			qtyMemory := resourceQuota.Spec.Hard["requests.memory"]
			Expect((&qtyMemory).Cmp(expectedMemory)).To(Equal(0))

			expectedStorage := resource.MustParse("10Gi")
			qtyStorage := resourceQuota.Spec.Hard["requests.storage"]
			Expect((&qtyStorage).Cmp(expectedStorage)).To(Equal(0))

			expectedPVCs := resource.MustParse("1")
			qtyPVCs := resourceQuota.Spec.Hard["persistentvolumeclaims"]
			Expect((&qtyPVCs).Cmp(expectedPVCs)).To(Equal(0))

			expectedPods := resource.MustParse("5")
			qtyPods := resourceQuota.Spec.Hard["pods"]
			Expect((&qtyPods).Cmp(expectedPods)).To(Equal(0))

			limitRange := &corev1.LimitRange{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "costguard-limitrange",
				Namespace: "managed-test-namespace",
			}, limitRange)).To(Succeed())
			Expect(limitRange.Spec.Limits).To(HaveLen(1))
			item := limitRange.Spec.Limits[0]
			Expect(item.Type).To(Equal(corev1.LimitTypeContainer))

			expectedReqCPU := resource.MustParse("100m")
			qtyReqCPU := item.DefaultRequest["cpu"]
			Expect((&qtyReqCPU).Cmp(expectedReqCPU)).To(Equal(0))

			expectedReqMemory := resource.MustParse("128Mi")
			qtyReqMemory := item.DefaultRequest["memory"]
			Expect((&qtyReqMemory).Cmp(expectedReqMemory)).To(Equal(0))

			expectedLimitCPU := resource.MustParse("250m")
			qtyLimitCPU := item.Default["cpu"]
			Expect((&qtyLimitCPU).Cmp(expectedLimitCPU)).To(Equal(0))

			expectedLimitMemory := resource.MustParse("256Mi")
			qtyLimitMemory := item.Default["memory"]
			Expect((&qtyLimitMemory).Cmp(expectedLimitMemory)).To(Equal(0))

			reconciled := &finopsv1alpha1.BudgetNamespace{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, reconciled)).To(Succeed())
			Expect(reconciled.Status.ManagedNamespace).To(Equal("managed-test-namespace"))
			Expect(reconciled.Status.ObservedGeneration).To(Equal(reconciled.Generation))
			Expect(reconciled.Status.ExpiresAt).NotTo(BeNil())
			expiredCondition := meta.FindStatusCondition(reconciled.Status.Conditions, "Expired")
			Expect(expiredCondition).NotTo(BeNil())
			Expect(expiredCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(expiredCondition.Reason).To(Equal("TTLActive"))
			Expect(reconciled.Status.Conditions).NotTo(BeEmpty())
			readyCondition := meta.FindStatusCondition(reconciled.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal("NamespaceReady"))
		})

		It("should delete the namespace when TTL is expired and grace elapsed", func() {
			const expiredCRName = "test-resource-expired"
			const expiredManagedNamespaceName = "managed-test-namespace-expired"

			By("creating the managed Namespace")
			expiredNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: expiredManagedNamespaceName,
				},
			}
			Expect(k8sClient.Create(ctx, expiredNamespace)).To(Succeed())

			By("creating the BudgetNamespace with TTL already expired")
			expiredBudgetNamespace := &finopsv1alpha1.BudgetNamespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      expiredCRName,
					Namespace: "default",
				},
				Spec: finopsv1alpha1.BudgetNamespaceSpec{
					NamespaceName: expiredManagedNamespaceName,
					TTL:           "-1h",
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
			Expect(k8sClient.Create(ctx, expiredBudgetNamespace)).To(Succeed())

			By("reconciling")
			controllerReconciler := &BudgetNamespaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      expiredCRName,
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("verifying namespace deletion started")
			managedNs := &corev1.Namespace{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: expiredManagedNamespaceName}, managedNs)
			if err != nil && errors.IsNotFound(err) {
				// Namespace already gone; acceptable for envtest.
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(managedNs.DeletionTimestamp).NotTo(BeNil())
			}

			By("verifying status conditions")
			reconciled := &finopsv1alpha1.BudgetNamespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      expiredCRName,
				Namespace: "default",
			}, reconciled)).To(Succeed())

			expiredCondition := meta.FindStatusCondition(reconciled.Status.Conditions, "Expired")
			Expect(expiredCondition).NotTo(BeNil())
			Expect(expiredCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(expiredCondition.Reason).To(Equal("TTLExpired"))

			readyCondition := meta.FindStatusCondition(reconciled.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("TTLExpired"))

			By("cleanup")
			// Ignore cleanup errors; controller may race.
			_ = k8sClient.Delete(ctx, expiredBudgetNamespace)
		})
	})
})
