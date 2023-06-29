/*
Copyright 2023.

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
	"time"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AlertSpec defines the desired state of Alert
type AlertSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ResourceGVK describes the GVK to watch changes for
	ResourceGVK ResourceGVK `json:"resourceGVK,omitempty"`

	// ObjectFilters help users filter across all objects of given GVK
	ObjectFilters []ObjectFilter `json:"objectFilters,omitempty"`

	// Condition represents when to send notification
	Condition Condition `json:"condition"`
}

type ResourceGVK struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

type ObjectFilterOperator string

const (
	ObjectFilterOperatorIn  ObjectFilterOperator = "In"
	ObjectFilterOperatorEq  ObjectFilterOperator = "Eq"
	ObjectFilterOperatorNot ObjectFilterOperator = "Not"
)

type ObjectFilter struct {
	Path     string               `json:"path"`
	Operator ObjectFilterOperator `json:"operator"`
	Value    string               `json:"value"`
}

type Condition struct {
	Expression string        `json:"expression"`
	For        time.Duration `json:"for"`
}

// AlertStatus defines the observed state of Alert
type AlertStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Alert is the Schema for the alerts API
type Alert struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AlertSpec   `json:"spec,omitempty"`
	Status AlertStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AlertList contains a list of Alert
type AlertList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Alert `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Alert{}, &AlertList{})
}
