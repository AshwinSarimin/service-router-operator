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

// GatewaySpec defines the desired state of Gateway
type GatewaySpec struct {
	// Controller is the Istio gateway implementation (e.g., "aks-istio-ingressgateway-internal")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Controller string `json:"controller"`

	// CredentialName is the TLS certificate secret name
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	CredentialName string `json:"credentialName"`

	// TargetPostfix is the postfix used in target hostname (e.g., "external", "internal")
	// This is appended to {cluster}-{region}- to form the complete target host
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	TargetPostfix string `json:"targetPostfix"`
}

// GatewayStatus defines the observed state of Gateway
type GatewayStatus struct {
	// Phase represents the current phase (Pending, Active, Failed)
	// +kubebuilder:validation:Enum=Pending;Active;Failed
	Phase string `json:"phase,omitempty"`

	// LoadBalancerIP is the external IP of the gateway service
	// +optional
	LoadBalancerIP string `json:"loadBalancerIP,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=gw
// +kubebuilder:storageversion

// Gateway is the Schema for the gateways API
type Gateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewaySpec   `json:"spec,omitempty"`
	Status GatewayStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayList contains a list of Gateway
type GatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Gateway{}, &GatewayList{})
}
