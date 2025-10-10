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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExternalDNSController defines an ExternalDNS controller configuration
type ExternalDNSController struct {
	// Controller is the name of the ExternalDNS controller (e.g., "external-dns-neu")
	// +kubebuilder:validation:Required
	Controller string `json:"controller"`
	
	// Region is the region this controller manages (e.g., "neu", "weu")
	// +kubebuilder:validation:Required
	Region string `json:"region"`
}

// Gateway defines an Istio gateway configuration
type Gateway struct {
	// Name is the name of the gateway (e.g., "default-gateway-ingress")
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	
	// Controller is the Istio gateway controller name (e.g., "aks-istio-ingressgateway-internal")
	// +kubebuilder:validation:Required
	Controller string `json:"controller"`
	
	// CredentialName is the TLS certificate secret name
	// +kubebuilder:validation:Required
	CredentialName string `json:"credentialName"`
	
	// TargetPostfix is used in the target hostname (e.g., "external", "internal")
	// +kubebuilder:default:="external"
	TargetPostfix string `json:"targetPostfix,omitempty"`
	
	// HTTPSPortNumber is the HTTPS port number
	// +kubebuilder:default:=443
	HTTPSPortNumber int32 `json:"httpsPortNumber,omitempty"`
}

// Application defines an application and its services
type Application struct {
	// Name is the application name (e.g., "nid-02")
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	
	// Environment is the environment name (e.g., "dev", "test", "prod")
	// +kubebuilder:validation:Required
	Environment string `json:"environment"`
	
	// Services is a map of gateway names to service lists
	// Key: gateway name (must match a gateway in the gateways list)
	// Value: list of service names
	// +kubebuilder:validation:Required
	Services map[string][]string `json:"services"`
	
	// Mode defines how the app is deployed: "active" or "regionbound"
	// +kubebuilder:default:="active"
	// +kubebuilder:validation:Enum=active;regionbound
	Mode string `json:"mode,omitempty"`
	
	// Region is required when mode is "regionbound"
	// +kubebuilder:validation:Optional
	Region string `json:"region,omitempty"`
	
	// Cluster allows overriding the cluster name for this specific app
	// +kubebuilder:validation:Optional
	Cluster string `json:"cluster,omitempty"`
}

// ServiceRouterSpec defines the desired state of ServiceRouter
type ServiceRouterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Cluster is the current cluster name (e.g., "aks01")
	// +kubebuilder:validation:Required
	Cluster string `json:"cluster"`
	
	// Region is the current region code (e.g., "neu", "weu")
	// +kubebuilder:validation:Required
	Region string `json:"region"`
	
	// EnvironmentLetter is the environment letter (d=dev, t=test, p=prod)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern:="^[dtp]$"
	EnvironmentLetter string `json:"environmentLetter"`
	
	// Domain is the base domain for DNS records (e.g., "aks.vecd.vczc.nl")
	// +kubebuilder:validation:Required
	Domain string `json:"domain"`
	
	// ExternalDNS is the list of ExternalDNS controllers
	// +kubebuilder:validation:MinItems=1
	ExternalDNS []ExternalDNSController `json:"externalDns"`
	
	// Gateways is the list of Istio gateways
	// +kubebuilder:validation:MinItems=1
	Gateways []Gateway `json:"gateways"`
	
	// Apps is the list of applications and their services
	// +kubebuilder:validation:MinItems=1
	Apps []Application `json:"apps"`
}

// ServiceRouterStatus defines the observed state of ServiceRouter
type ServiceRouterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions represent the latest available observations of the ServiceRouter's state
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	
	// DNSEndpointsCreated is the number of DNSEndpoint resources created
	// +operator-sdk:csv:customresourcedefinitions:type=status
	DNSEndpointsCreated int32 `json:"dnsEndpointsCreated,omitempty"`
	
	// GatewaysCreated is the number of Gateway resources created
	// +operator-sdk:csv:customresourcedefinitions:type=status
	GatewaysCreated int32 `json:"gatewaysCreated,omitempty"`
	
	// LastReconciled is the timestamp of the last successful reconciliation
	// +operator-sdk:csv:customresourcedefinitions:type=status
	LastReconciled *metav1.Time `json:"lastReconciled,omitempty"`
	
	// ObservedGeneration represents the .metadata.generation that the status was set based upon
	// +operator-sdk:csv:customresourcedefinitions:type=status
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.cluster`
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.spec.region`
// +kubebuilder:printcolumn:name="DNS Endpoints",type=integer,JSONPath=`.status.dnsEndpointsCreated`
// +kubebuilder:printcolumn:name="Gateways",type=integer,JSONPath=`.status.gatewaysCreated`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

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