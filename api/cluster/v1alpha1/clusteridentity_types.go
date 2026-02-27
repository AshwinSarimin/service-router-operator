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

// ClusterIdentitySpec defines the desired state of ClusterIdentity
type ClusterIdentitySpec struct {
	// Region is the identifier for this cluster's region (e.g., "neu", "weu", "frc")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// Cluster is the identifier for this cluster (e.g., "aks01")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Cluster string `json:"cluster"`

	// Domain is the base domain for DNS records
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,}$`
	Domain string `json:"domain"`

	// EnvironmentLetter is the environment identifier (e.g., "d", "t", "p")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z]$`
	EnvironmentLetter string `json:"environmentLetter"`

	// AdoptsRegions is a list of orphan regions that this cluster should manage
	// +optional
	AdoptsRegions []string `json:"adoptsRegions,omitempty"`
}

// ClusterIdentityStatus defines the observed state of ClusterIdentity
type ClusterIdentityStatus struct {
	// Phase represents the current phase (Pending, Active, Failed)
	// +kubebuilder:validation:Enum=Pending;Active;Failed
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ci
// +kubebuilder:storageversion

// ClusterIdentity is the Schema for the clusteridentities API
type ClusterIdentity struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterIdentitySpec   `json:"spec,omitempty"`
	Status ClusterIdentityStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterIdentityList contains a list of ClusterIdentity
type ClusterIdentityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterIdentity `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterIdentity{}, &ClusterIdentityList{})
}
