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

// LogBackendSpec defines the desired state of LogBackend
type LogBackendSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of LogBackend. Edit logbackend_types.go to remove/update
	Foo string `json:"foo,omitempty"`

	BufferSize          int `json:"bufferSize"`
	FlushSecondInterval int `json:"flushSecondInterval"`
}

// LogBackendStatus defines the observed state of LogBackend
type LogBackendStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	SyncPhase      LogBackendSyncPhase `json:"syncPhase"`
	LastDeployTime *metav1.Time        `json:"lastDeployTime"`
}

type LogBackendSyncPhase string

const (
	StatusLogBackendPending  LogBackendSyncPhase = "Pending"
	StatusLogBackendRunning  LogBackendSyncPhase = "Running"
	StatusLogBackendDeleting LogBackendSyncPhase = "Deleting"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.syncPhase`
// +kubebuilder:printcolumn:name="LastDeployTime",type="date",JSONPath=".status.lastDeployTime"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:shortName=lb
// LogBackend is the Schema for the logbackends API
type LogBackend struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LogBackendSpec   `json:"spec,omitempty"`
	Status LogBackendStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LogBackendList contains a list of LogBackend
type LogBackendList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LogBackend `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LogBackend{}, &LogBackendList{})
}
