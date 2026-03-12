/*
Copyright 2025.

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

// DNSPolicySpec defines the desired state of DNSPolicy
type DNSPolicySpec struct {
	// Mode defines how DNS records are managed (Active, RegionBound)
	// +kubebuilder:validation:Enum=Active;RegionBound
	// +kubebuilder:default=Active
	Mode string `json:"mode,omitempty"`

	// SourceRegion specifies the region this policy is intended for
	// When set, the policy will only activate if the cluster's region matches
	// Used with RegionBound mode to prevent cross-cluster conflicts
	// If empty, the policy is considered active regardless of cluster region
	// +optional
	SourceRegion string `json:"sourceRegion,omitempty"`

	// SourceCluster specifies the cluster identifier this policy is intended for
	// When set, the policy will only activate if the cluster's identity matches
	// Provides additional safety beyond region matching
	// If empty, the policy is considered active regardless of cluster name
	// +optional
	SourceCluster string `json:"sourceCluster,omitempty"`
}

// DNSPolicyStatus defines the observed state of DNSPolicy
type DNSPolicyStatus struct {
	// Phase represents the current phase (Pending, Active, Failed, Inactive)
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Inactive
	Phase string `json:"phase,omitempty"`

	// Active indicates if this policy is currently active based on cluster identity
	// A policy is inactive if sourceRegion or sourceCluster don't match the current cluster
	Active bool `json:"active,omitempty"`

	// ActiveControllers lists controllers currently managing records
	// Will be empty if the policy is not active
	ActiveControllers []string `json:"activeControllers,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=dnsp
// +kubebuilder:storageversion

// DNSPolicy is the Schema for the dnspolicies API
type DNSPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DNSPolicySpec   `json:"spec,omitempty"`
	Status DNSPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DNSPolicyList contains a list of DNSPolicy
type DNSPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DNSPolicy{}, &DNSPolicyList{})
}
