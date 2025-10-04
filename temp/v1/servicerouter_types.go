package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceRouterSpec defines the desired state of ServiceRouter
type ServiceRouterSpec struct {
	// Add custom resource fields here
	// Example: Replicas int32 `json:"replicas,omitempty"`
}

// ServiceRouterStatus defines the observed state of ServiceRouter
type ServiceRouterStatus struct {
	// Add custom resource status fields here
	// Example: AvailableReplicas int32 `json:"availableReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ServiceRouter is the Schema for the servicerouters API
type ServiceRouter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceRouterSpec   `json:"spec,omitempty"`
	Status ServiceRouterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceRouterList contains a list of ServiceRouter
type ServiceRouterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceRouter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceRouter{}, &ServiceRouterList{})
}