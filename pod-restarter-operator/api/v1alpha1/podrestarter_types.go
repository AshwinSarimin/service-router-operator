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

// PodRestarterSpec defines the desired state of PodRestarter
type PodRestarterSpec struct {
	// Selector is the label selector to find pods to restart
	// +kubebuilder:validation:Required
	Selector metav1.LabelSelector `json:"selector"`

	// IntervalMinutes is how often to restart pods (in minutes)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1440
	// +kubebuilder:default=5
	IntervalMinutes int32 `json:"intervalMinutes,omitempty"`

	// Strategy defines how to restart pods
	// - "all": Restart all matching pods at once
	// - "rolling": Restart one pod at a time
	// - "random-one": Restart one random pod
	// +kubebuilder:validation:Enum=all;rolling;random-one
	// +kubebuilder:default="all"
	Strategy string `json:"strategy,omitempty"`

	// MaxConcurrent limits how many pods to restart at once
	// 0 means no limit
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	MaxConcurrent int32 `json:"maxConcurrent,omitempty"`

	// Suspend will pause pod restarts when true
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`
}

// PodRestarterStatus defines the observed state of PodRestarter.
type PodRestarterStatus struct {
	// TotalRestarts is the total number of pod restarts performed
	TotalRestarts int32 `json:"totalRestarts,omitempty"`

	// LastRestartTime is when pods were last restarted
	LastRestartTime *metav1.Time `json:"lastRestartTime,omitempty"`

	// NextRestartTime is when pods will be restarted next
	NextRestartTime *metav1.Time `json:"nextRestartTime,omitempty"`

	// MatchingPods is the current number of pods matching the selector
	MatchingPods int32 `json:"matchingPods,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed PodRestarter
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Strategy",type=string,JSONPath=`.spec.strategy`
// +kubebuilder:printcolumn:name="Interval",type=integer,JSONPath=`.spec.intervalMinutes`
// +kubebuilder:printcolumn:name="Matching Pods",type=integer,JSONPath=`.status.matchingPods`
// +kubebuilder:printcolumn:name="Total Restarts",type=integer,JSONPath=`.status.totalRestarts`
// +kubebuilder:printcolumn:name="Last Restart",type=date,JSONPath=`.status.lastRestartTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PodRestarter is the Schema for the podrestarters API
type PodRestarter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodRestarterSpec   `json:"spec,omitempty"`
	Status PodRestarterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PodRestarterList contains a list of PodRestarter
type PodRestarterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodRestarter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodRestarter{}, &PodRestarterList{})
}
