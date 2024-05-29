/*
Copyright 2024.

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

// OktaGroupSpec defines the desired state of OktaGroup
type OktaGroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Description is the description of the Okta group
	Description string `json:"description,omitempty"`
	// Users is the list of users in the Okta group
	Users []string `json:"users"`
}

// OktaGroupStatus defines the observed state of OktaGroup
type OktaGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// These fields are automatically set by the system.

	// Created is the time when the Okta group was created.
	Created metav1.Time `json:"created,omitempty"`
	// Id is the unique identifier of the Okta group.
	Id string `json:"id,omitempty"`
	// LastMembershipUpdated is the time when the membership of the Okta group was last updated.
	LastMembershipUpdated metav1.Time `json:"lastMembershipUpdated,omitempty"`
	// LastUpdated is the time when the Okta group was last updated.
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// OktaGroup is the Schema for the oktagroups API
type OktaGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OktaGroupSpec   `json:"spec,omitempty"`
	Status OktaGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OktaGroupList contains a list of OktaGroup
type OktaGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OktaGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OktaGroup{}, &OktaGroupList{})
}
