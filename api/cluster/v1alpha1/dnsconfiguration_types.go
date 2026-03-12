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

// DNSConfigurationSpec defines the desired state of DNSConfiguration
type DNSConfigurationSpec struct {
	// ExternalDNSControllers lists all ExternalDNS controllers in the infrastructure
	// +kubebuilder:validation:MinItems=1
	ExternalDNSControllers []ExternalDNSController `json:"externalDNSControllers"`
}

// ExternalDNSController defines an ExternalDNS controller configuration
type ExternalDNSController struct {
	// Name is the controller identifier (e.g., "external-dns-neu")
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Region is the region this controller is responsible for
	// In RegionBound mode, only controllers matching the cluster's region will be active
	// +kubebuilder:validation:Required
	Region string `json:"region"`
}

// DNSConfigurationStatus defines the observed state of DNSConfiguration
type DNSConfigurationStatus struct {
	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:storageversion

// DNSConfiguration is the Schema for the dnsconfigurations API
type DNSConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DNSConfigurationSpec   `json:"spec,omitempty"`
	Status DNSConfigurationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DNSConfigurationList contains a list of DNSConfiguration
type DNSConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DNSConfiguration{}, &DNSConfigurationList{})
}
