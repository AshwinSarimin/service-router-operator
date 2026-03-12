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

// ServiceRouteSpec defines the desired state of ServiceRoute
type ServiceRouteSpec struct {
	// ServiceName is the name of the service (used in DNS)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ServiceName string `json:"serviceName"`

	// GatewayName is the name of the Gateway resource to use
	// References a namespace-scoped Gateway resource
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	GatewayName string `json:"gatewayName"`

	// GatewayNamespace is the namespace where the Gateway resource is located
	// +kubebuilder:validation:Optional
	GatewayNamespace string `json:"gatewayNamespace,omitempty"`

	// Environment is the environment name (e.g., "dev", "test", "prod")
	// +kubebuilder:validation:Required
	Environment string `json:"environment"`

	// Application is the application name (used in DNS)
	// +kubebuilder:validation:Required
	Application string `json:"application"`
}

// ServiceRouteStatus defines the observed state of ServiceRoute
type ServiceRouteStatus struct {
	// Phase represents the current phase (Pending, Active, Failed)
	// +kubebuilder:validation:Enum=Pending;Active;Failed
	Phase string `json:"phase,omitempty"`

	// DNSEndpoint is the name of the generated DNSEndpoint resource
	DNSEndpoint string `json:"dnsEndpoint,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=sr
// +kubebuilder:storageversion

// ServiceRoute is the Schema for the serviceroutes API
type ServiceRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceRouteSpec   `json:"spec,omitempty"`
	Status ServiceRouteStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServiceRouteList contains a list of ServiceRoute
type ServiceRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceRoute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceRoute{}, &ServiceRouteList{})
}
