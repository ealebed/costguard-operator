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
	"maps"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	finopsv1alpha1 "github.com/ealebed/costguard-operator/api/v1alpha1"
)

const (
	testBudgetCRNamespace        = "default"
	testManagedNamespaceName     = "managed-test-namespace"
	testQuotaMemory              = "1Gi"
	testQuotaStorage             = "10Gi"
	testDefaultsRequestCPU       = "100m"
	testDefaultsRequestMemory    = "128Mi"
	testDefaultsLimitCPU         = "250m"
	testDefaultsLimitMemory      = "256Mi"
	testWorkloadName             = "workload"
	testWorkloadSelectorLabelKey = "app"
	testContainerImage           = "pause:latest"
	testLongTTL                  = "1000h"
)

type fakeSpendQuerier struct {
	v   float64
	err error
}

func (f *fakeSpendQuerier) NamespaceSpendUSD(
	context.Context, string, string, string, string, time.Duration,
) (float64, error) {
	return f.v, f.err
}

var _ = Describe("BudgetNamespace Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: testBudgetCRNamespace, // TODO(user):Modify as needed
		}
		budgetnamespace := &finopsv1alpha1.BudgetNamespace{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind BudgetNamespace")
			err := k8sClient.Get(ctx, typeNamespacedName, budgetnamespace)
			if err != nil && errors.IsNotFound(err) {
				resource := &finopsv1alpha1.BudgetNamespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: testBudgetCRNamespace,
					},
					Spec: finopsv1alpha1.BudgetNamespaceSpec{
						NamespaceName: testManagedNamespaceName,
						Labels: map[string]string{
							"ealebed.github.io/team": "platform",
						},
						Annotations: map[string]string{
							"ealebed.github.io/owner": "ealebed",
						},
						Quota: finopsv1alpha1.BudgetNamespaceQuotaSpec{
							CPU:                    "1",
							Memory:                 testQuotaMemory,
							Storage:                testQuotaStorage,
							PersistentVolumeClaims: 1,
							Pods:                   5,
						},
						Defaults: finopsv1alpha1.BudgetNamespaceDefaultsSpec{
							RequestCPU:    testDefaultsRequestCPU,
							RequestMemory: testDefaultsRequestMemory,
							LimitCPU:      testDefaultsLimitCPU,
							LimitMemory:   testDefaultsLimitMemory,
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
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testManagedNamespaceName}, namespace)).To(Succeed())

			Expect(namespace.Labels["ealebed.github.io/team"]).To(Equal("platform"))
			Expect(namespace.Annotations["ealebed.github.io/owner"]).To(Equal("ealebed"))

			resourceQuota := &corev1.ResourceQuota{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "costguard-quota",
				Namespace: testManagedNamespaceName,
			}, resourceQuota)).To(Succeed())
			expectedCPU := resource.MustParse("1")
			qtyCPU := resourceQuota.Spec.Hard[corev1.ResourceRequestsCPU]
			Expect((&qtyCPU).Cmp(expectedCPU)).To(Equal(0))

			expectedMemory := resource.MustParse("1Gi")
			qtyMemory := resourceQuota.Spec.Hard[corev1.ResourceRequestsMemory]
			Expect((&qtyMemory).Cmp(expectedMemory)).To(Equal(0))

			expectedStorage := resource.MustParse("10Gi")
			qtyStorage := resourceQuota.Spec.Hard[corev1.ResourceRequestsStorage]
			Expect((&qtyStorage).Cmp(expectedStorage)).To(Equal(0))

			expectedPVCs := resource.MustParse("1")
			qtyPVCs := resourceQuota.Spec.Hard[corev1.ResourcePersistentVolumeClaims]
			Expect((&qtyPVCs).Cmp(expectedPVCs)).To(Equal(0))

			expectedPods := resource.MustParse("5")
			qtyPods := resourceQuota.Spec.Hard[corev1.ResourcePods]
			Expect((&qtyPods).Cmp(expectedPods)).To(Equal(0))

			limitRange := &corev1.LimitRange{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "costguard-limitrange",
				Namespace: testManagedNamespaceName,
			}, limitRange)).To(Succeed())
			Expect(limitRange.Spec.Limits).To(HaveLen(1))
			item := limitRange.Spec.Limits[0]
			Expect(item.Type).To(Equal(corev1.LimitTypeContainer))

			expectedReqCPU := resource.MustParse("100m")
			qtyReqCPU := item.DefaultRequest[corev1.ResourceCPU]
			Expect((&qtyReqCPU).Cmp(expectedReqCPU)).To(Equal(0))

			expectedReqMemory := resource.MustParse("128Mi")
			qtyReqMemory := item.DefaultRequest[corev1.ResourceMemory]
			Expect((&qtyReqMemory).Cmp(expectedReqMemory)).To(Equal(0))

			expectedLimitCPU := resource.MustParse("250m")
			qtyLimitCPU := item.Default[corev1.ResourceCPU]
			Expect((&qtyLimitCPU).Cmp(expectedLimitCPU)).To(Equal(0))

			expectedLimitMemory := resource.MustParse("256Mi")
			qtyLimitMemory := item.Default[corev1.ResourceMemory]
			Expect((&qtyLimitMemory).Cmp(expectedLimitMemory)).To(Equal(0))

			reconciled := &finopsv1alpha1.BudgetNamespace{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, reconciled)).To(Succeed())
			Expect(reconciled.Status.ManagedNamespace).To(Equal(testManagedNamespaceName))
			Expect(reconciled.Status.ObservedGeneration).To(Equal(reconciled.Generation))
			Expect(reconciled.Status.ExpiresAt).NotTo(BeNil())
			expiredCondition := meta.FindStatusCondition(reconciled.Status.Conditions, conditionTypeExpired)
			Expect(expiredCondition).NotTo(BeNil())
			Expect(expiredCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(expiredCondition.Reason).To(Equal("TTLActive"))
			Expect(reconciled.Status.Conditions).NotTo(BeEmpty())
			readyCondition := meta.FindStatusCondition(reconciled.Status.Conditions, conditionTypeReady)
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
					Namespace: testBudgetCRNamespace,
				},
				Spec: finopsv1alpha1.BudgetNamespaceSpec{
					NamespaceName: expiredManagedNamespaceName,
					TTL:           "-1h",
					Quota: finopsv1alpha1.BudgetNamespaceQuotaSpec{
						CPU:                    "1",
						Memory:                 testQuotaMemory,
						Storage:                testQuotaStorage,
						PersistentVolumeClaims: 1,
						Pods:                   5,
					},
					Defaults: finopsv1alpha1.BudgetNamespaceDefaultsSpec{
						RequestCPU:    testDefaultsRequestCPU,
						RequestMemory: testDefaultsRequestMemory,
						LimitCPU:      testDefaultsLimitCPU,
						LimitMemory:   testDefaultsLimitMemory,
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
					Namespace: testBudgetCRNamespace,
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
				Namespace: testBudgetCRNamespace,
			}, reconciled)).To(Succeed())

			expiredCondition := meta.FindStatusCondition(reconciled.Status.Conditions, conditionTypeExpired)
			Expect(expiredCondition).NotTo(BeNil())
			Expect(expiredCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(expiredCondition.Reason).To(Equal(conditionReasonTTLExpired))

			readyCondition := meta.FindStatusCondition(reconciled.Status.Conditions, conditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal(conditionReasonTTLExpired))

			By("cleanup")
			// Ignore cleanup errors; controller may race.
			_ = k8sClient.Delete(ctx, expiredBudgetNamespace)
		})

		It("should scale Deployments to zero during TTL grace when enforcement requests ScaleToZero", func() {
			const graceCRName = "test-resource-grace-scale"
			const graceManagedNS = "managed-test-namespace-grace"

			replicas := int32(2)
			By("creating the managed Namespace and a Deployment")
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: graceManagedNS},
			})).To(Succeed())

			Expect(k8sClient.Create(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkloadName,
					Namespace: graceManagedNS,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{testWorkloadSelectorLabelKey: testWorkloadName},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{testWorkloadSelectorLabelKey: testWorkloadName},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "c",
								Image: testContainerImage,
							}},
						},
					},
				},
			})).To(Succeed())

			By("creating BudgetNamespace with short TTL and scale-to-zero enforcement")
			graceBN := &finopsv1alpha1.BudgetNamespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      graceCRName,
					Namespace: testBudgetCRNamespace,
				},
				Spec: finopsv1alpha1.BudgetNamespaceSpec{
					NamespaceName: graceManagedNS,
					TTL:           "1s",
					Enforcement: finopsv1alpha1.BudgetNamespaceEnforcementSpec{
						Enabled: true,
						Action:  enforcementOpScaleToZero,
					},
					Quota: finopsv1alpha1.BudgetNamespaceQuotaSpec{
						CPU:                    "1",
						Memory:                 testQuotaMemory,
						Storage:                testQuotaStorage,
						PersistentVolumeClaims: 1,
						Pods:                   5,
					},
					Defaults: finopsv1alpha1.BudgetNamespaceDefaultsSpec{
						RequestCPU:    testDefaultsRequestCPU,
						RequestMemory: testDefaultsRequestMemory,
						LimitCPU:      testDefaultsLimitCPU,
						LimitMemory:   testDefaultsLimitMemory,
					},
				},
			}
			Expect(k8sClient.Create(ctx, graceBN)).To(Succeed())

			By("waiting until TTL expired but grace period not elapsed")
			time.Sleep(2 * time.Second)

			rec := &BudgetNamespaceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := rec.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: graceCRName, Namespace: testBudgetCRNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testWorkloadName, Namespace: graceManagedNS}, deploy)).To(Succeed())
			Expect(deploy.Spec.Replicas).NotTo(BeNil())
			Expect(*deploy.Spec.Replicas).To(Equal(int32(0)))
			Expect(deploy.Annotations["finops.ealebed.github.io/pre-scale-replicas"]).To(Equal("2"))

			By("cleanup")
			_ = k8sClient.Delete(ctx, graceBN)
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: graceManagedNS}})
		})

		It("should scale Deployments to zero when ResourceQuota usage reaches hard limits", func() {
			const quotaCRName = "test-resource-quota-scale"
			const quotaManagedNS = "managed-quota-rq-ns"

			replicas := int32(2)
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: quotaManagedNS},
			})).To(Succeed())

			quotaBN := &finopsv1alpha1.BudgetNamespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      quotaCRName,
					Namespace: testBudgetCRNamespace,
				},
				Spec: finopsv1alpha1.BudgetNamespaceSpec{
					NamespaceName: quotaManagedNS,
					TTL:           testLongTTL,
					Enforcement: finopsv1alpha1.BudgetNamespaceEnforcementSpec{
						Enabled: true,
						Action:  enforcementOpScaleToZero,
					},
					Quota: finopsv1alpha1.BudgetNamespaceQuotaSpec{
						CPU:                    "1",
						Memory:                 testQuotaMemory,
						Storage:                testQuotaStorage,
						PersistentVolumeClaims: 1,
						Pods:                   5,
					},
					Defaults: finopsv1alpha1.BudgetNamespaceDefaultsSpec{
						RequestCPU:    testDefaultsRequestCPU,
						RequestMemory: testDefaultsRequestMemory,
						LimitCPU:      testDefaultsLimitCPU,
						LimitMemory:   testDefaultsLimitMemory,
					},
				},
			}
			Expect(k8sClient.Create(ctx, quotaBN)).To(Succeed())

			rec := &BudgetNamespaceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := rec.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: quotaCRName, Namespace: testBudgetCRNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Create(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkloadName,
					Namespace: quotaManagedNS,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{testWorkloadSelectorLabelKey: testWorkloadName},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{testWorkloadSelectorLabelKey: testWorkloadName},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "c",
								Image: testContainerImage,
							}},
						},
					},
				},
			})).To(Succeed())

			rq := &corev1.ResourceQuota{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resourceQuotaName, Namespace: quotaManagedNS,
			}, rq)).To(Succeed())
			used := make(corev1.ResourceList, len(rq.Spec.Hard))
			maps.Copy(used, rq.Spec.Hard)
			rq.Status.Used = used
			Expect(k8sClient.Status().Update(ctx, rq)).To(Succeed())

			_, err = rec.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: quotaCRName, Namespace: testBudgetCRNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testWorkloadName, Namespace: quotaManagedNS}, deploy)).To(Succeed())
			Expect(deploy.Spec.Replicas).NotTo(BeNil())
			Expect(*deploy.Spec.Replicas).To(Equal(int32(0)))
			Expect(deploy.Annotations["finops.ealebed.github.io/pre-scale-replicas"]).To(Equal("2"))

			reconciled := &finopsv1alpha1.BudgetNamespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: quotaCRName, Namespace: testBudgetCRNamespace}, reconciled)).To(Succeed())
			ob := meta.FindStatusCondition(reconciled.Status.Conditions, conditionTypeOverBudget)
			Expect(ob).NotTo(BeNil())
			Expect(ob.Status).To(Equal(metav1.ConditionTrue))
			Expect(ob.Reason).To(Equal("ResourceQuotaAtOrOverHard"))

			By("cleanup")
			_ = k8sClient.Delete(ctx, quotaBN)
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: quotaManagedNS}})
		})

		It("should restore Deployments when quota is below hard and restoreOnRecovery is set", func() {
			const restoreCRName = "test-resource-quota-restore"
			const restoreManagedNS = "managed-quota-restore-ns"

			replicas := int32(2)
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: restoreManagedNS},
			})).To(Succeed())

			restoreBN := &finopsv1alpha1.BudgetNamespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      restoreCRName,
					Namespace: testBudgetCRNamespace,
				},
				Spec: finopsv1alpha1.BudgetNamespaceSpec{
					NamespaceName: restoreManagedNS,
					TTL:           testLongTTL,
					Enforcement: finopsv1alpha1.BudgetNamespaceEnforcementSpec{
						Enabled:             true,
						Action:              enforcementOpScaleToZero,
						RestoreOnRecovery:   true,
						EnforcementCooldown: "0s",
					},
					Quota: finopsv1alpha1.BudgetNamespaceQuotaSpec{
						CPU:                    "1",
						Memory:                 testQuotaMemory,
						Storage:                testQuotaStorage,
						PersistentVolumeClaims: 1,
						Pods:                   5,
					},
					Defaults: finopsv1alpha1.BudgetNamespaceDefaultsSpec{
						RequestCPU:    testDefaultsRequestCPU,
						RequestMemory: testDefaultsRequestMemory,
						LimitCPU:      testDefaultsLimitCPU,
						LimitMemory:   testDefaultsLimitMemory,
					},
				},
			}
			Expect(k8sClient.Create(ctx, restoreBN)).To(Succeed())

			rec := &BudgetNamespaceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := rec.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: restoreCRName, Namespace: testBudgetCRNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Create(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkloadName,
					Namespace: restoreManagedNS,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{testWorkloadSelectorLabelKey: testWorkloadName},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{testWorkloadSelectorLabelKey: testWorkloadName},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "c",
								Image: testContainerImage,
							}},
						},
					},
				},
			})).To(Succeed())

			rq := &corev1.ResourceQuota{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resourceQuotaName, Namespace: restoreManagedNS,
			}, rq)).To(Succeed())
			usedHigh := make(corev1.ResourceList, len(rq.Spec.Hard))
			maps.Copy(usedHigh, rq.Spec.Hard)
			rq.Status.Used = usedHigh
			Expect(k8sClient.Status().Update(ctx, rq)).To(Succeed())

			_, err = rec.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: restoreCRName, Namespace: testBudgetCRNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testWorkloadName, Namespace: restoreManagedNS}, deploy)).To(Succeed())
			Expect(*deploy.Spec.Replicas).To(Equal(int32(0)))

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resourceQuotaName, Namespace: restoreManagedNS,
			}, rq)).To(Succeed())
			rq.Status.Used = corev1.ResourceList{
				corev1.ResourcePods:                   resource.MustParse("0"),
				corev1.ResourceRequestsCPU:            resource.MustParse("0"),
				corev1.ResourceRequestsMemory:         resource.MustParse("0"),
				corev1.ResourceRequestsStorage:        resource.MustParse("0"),
				corev1.ResourcePersistentVolumeClaims: resource.MustParse("0"),
			}
			Expect(k8sClient.Status().Update(ctx, rq)).To(Succeed())

			_, err = rec.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: restoreCRName, Namespace: testBudgetCRNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testWorkloadName, Namespace: restoreManagedNS}, deploy)).To(Succeed())
			Expect(*deploy.Spec.Replicas).To(Equal(int32(2)))
			Expect(deploy.Annotations).NotTo(HaveKey("finops.ealebed.github.io/pre-scale-replicas"))

			reconciled := &finopsv1alpha1.BudgetNamespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: restoreCRName, Namespace: testBudgetCRNamespace}, reconciled)).To(Succeed())
			Expect(reconciled.Status.LastEnforcementOperation).To(Equal(enforcementOpRestore))

			By("cleanup")
			_ = k8sClient.Delete(ctx, restoreBN)
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: restoreManagedNS}})
		})

		It("should scale Deployments to zero when cost budget spend exceeds maxSpendUSD", func() {
			const costCRName = "test-resource-cost-budget"
			const costManagedNS = "managed-cost-budget-ns"

			replicas := int32(2)
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: costManagedNS},
			})).To(Succeed())

			costBN := &finopsv1alpha1.BudgetNamespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      costCRName,
					Namespace: testBudgetCRNamespace,
				},
				Spec: finopsv1alpha1.BudgetNamespaceSpec{
					NamespaceName: costManagedNS,
					TTL:           testLongTTL,
					Enforcement: finopsv1alpha1.BudgetNamespaceEnforcementSpec{
						Enabled: true,
						Action:  enforcementOpScaleToZero,
					},
					Quota: finopsv1alpha1.BudgetNamespaceQuotaSpec{
						CPU:                    "1",
						Memory:                 testQuotaMemory,
						Storage:                testQuotaStorage,
						PersistentVolumeClaims: 1,
						Pods:                   5,
					},
					Defaults: finopsv1alpha1.BudgetNamespaceDefaultsSpec{
						RequestCPU:    testDefaultsRequestCPU,
						RequestMemory: testDefaultsRequestMemory,
						LimitCPU:      testDefaultsLimitCPU,
						LimitMemory:   testDefaultsLimitMemory,
					},
					CostBudget: &finopsv1alpha1.BudgetNamespaceCostBudgetSpec{
						Enabled:            true,
						BillingExportTable: "proj.dataset.table",
						ClusterName:        "test-cluster",
						MaxSpendUSD:        "1",
					},
				},
			}
			Expect(k8sClient.Create(ctx, costBN)).To(Succeed())

			rec := &BudgetNamespaceReconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				SpendQuerier: &fakeSpendQuerier{v: 2.0},
			}
			_, err := rec.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: costCRName, Namespace: testBudgetCRNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Create(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkloadName,
					Namespace: costManagedNS,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{testWorkloadSelectorLabelKey: testWorkloadName},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{testWorkloadSelectorLabelKey: testWorkloadName},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "c",
								Image: testContainerImage,
							}},
						},
					},
				},
			})).To(Succeed())

			_, err = rec.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: costCRName, Namespace: testBudgetCRNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testWorkloadName, Namespace: costManagedNS}, deploy)).To(Succeed())
			Expect(deploy.Spec.Replicas).NotTo(BeNil())
			Expect(*deploy.Spec.Replicas).To(Equal(int32(0)))
			Expect(deploy.Annotations["finops.ealebed.github.io/pre-scale-replicas"]).To(Equal("2"))

			reconciled := &finopsv1alpha1.BudgetNamespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: costCRName, Namespace: testBudgetCRNamespace}, reconciled)).To(Succeed())
			ob := meta.FindStatusCondition(reconciled.Status.Conditions, conditionTypeOverBudget)
			Expect(ob).NotTo(BeNil())
			Expect(ob.Status).To(Equal(metav1.ConditionTrue))
			Expect(ob.Reason).To(Equal("CostBudgetExceeded"))

			By("cleanup")
			_ = k8sClient.Delete(ctx, costBN)
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: costManagedNS}})
		})
	})
})
