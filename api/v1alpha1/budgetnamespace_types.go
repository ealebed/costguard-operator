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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BudgetNamespaceQuotaSpec defines the ResourceQuota values for the managed namespace.
type BudgetNamespaceQuotaSpec struct {
	// +kubebuilder:validation:MinLength=1
	CPU string `json:"cpu"`

	// +kubebuilder:validation:MinLength=1
	Memory string `json:"memory"`

	// +kubebuilder:validation:MinLength=1
	Storage string `json:"storage"`

	// +kubebuilder:validation:Minimum=1
	PersistentVolumeClaims int32 `json:"persistentVolumeClaims"`

	// +kubebuilder:validation:Minimum=1
	Pods int32 `json:"pods"`
}

// BudgetNamespaceDefaultsSpec defines default requests and limits via LimitRange.
type BudgetNamespaceDefaultsSpec struct {
	// +kubebuilder:validation:MinLength=1
	RequestCPU string `json:"requestCPU"`

	// +kubebuilder:validation:MinLength=1
	RequestMemory string `json:"requestMemory"`

	// +kubebuilder:validation:MinLength=1
	LimitCPU string `json:"limitCPU"`

	// +kubebuilder:validation:MinLength=1
	LimitMemory string `json:"limitMemory"`
}

// BudgetNamespaceEnforcementSpec defines what to do when the namespace exceeds budget.
type BudgetNamespaceEnforcementSpec struct {
	// +kubebuilder:default:=true
	Enabled bool `json:"enabled,omitempty"`

	// +kubebuilder:validation:Enum=None;ScaleToZero
	// +kubebuilder:default:=ScaleToZero
	Action string `json:"action,omitempty"`
}

// BudgetNamespaceSpec defines the desired state of BudgetNamespace
type BudgetNamespaceSpec struct {
	// namespaceName is the namespace managed by this BudgetNamespace resource.
	// +kubebuilder:validation:MinLength=1
	NamespaceName string `json:"namespaceName"`

	// labels are applied to the managed namespace.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations are applied to the managed namespace.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// quota defines ResourceQuota limits for the managed namespace.
	Quota BudgetNamespaceQuotaSpec `json:"quota"`

	// defaults defines default requests and limits for containers in the managed namespace.
	Defaults BudgetNamespaceDefaultsSpec `json:"defaults"`

	// ttl defines how long the managed namespace may live, for example 72h.
	// +optional
	// +kubebuilder:validation:MinLength=1
	TTL string `json:"ttl,omitempty"`

	// enforcement defines how budget violations are handled.
	// +optional
	Enforcement BudgetNamespaceEnforcementSpec `json:"enforcement,omitempty"`
}

// BudgetNamespaceStatus defines the observed state of BudgetNamespace.
type BudgetNamespaceStatus struct {
	// observedGeneration is the most recent generation reconciled by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// managedNamespace is the namespace currently managed for this resource.
	// +optional
	ManagedNamespace string `json:"managedNamespace,omitempty"`

	// expiresAt is the computed expiry timestamp derived from spec.ttl.
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the BudgetNamespace resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// BudgetNamespace is the Schema for the budgetnamespaces API
type BudgetNamespace struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of BudgetNamespace
	// +required
	Spec BudgetNamespaceSpec `json:"spec"`

	// status defines the observed state of BudgetNamespace
	// +optional
	Status BudgetNamespaceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// BudgetNamespaceList contains a list of BudgetNamespace
type BudgetNamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []BudgetNamespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BudgetNamespace{}, &BudgetNamespaceList{})
}
