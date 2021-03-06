/*
Copyright 2018 Datadog Inc..

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

type BirdcageSourceObject struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
}

type BirdcageTargetObject struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
	LuaCode   string `json:"luaCode"`
}

// BirdcageSpec defines the desired state of Birdcage
type BirdcageSpec struct {
	SourceObject BirdcageSourceObject `json:"sourceObject"`
	TargetObject BirdcageTargetObject `json:"targetObject"`
}

type BirdcageObjectRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// BirdcageStatus defines the observed state of Birdcage
type BirdcageStatus struct {
	Phase     string            `json:"phase,omitempty"`
	SourceRef BirdcageObjectRef `json:"sourceref,omitempty"`
	TargetRef BirdcageObjectRef `json:"targetref,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Birdcage is the Schema for the birdcages API
// +k8s:openapi-gen=true
type Birdcage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BirdcageSpec   `json:"spec,omitempty"`
	Status BirdcageStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BirdcageList contains a list of Birdcage
type BirdcageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Birdcage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Birdcage{}, &BirdcageList{})
}
