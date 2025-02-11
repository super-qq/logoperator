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

// CollectRuleSpec defines the desired state of CollectRule
type CollectRuleSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of CollectRule. Edit collectrule_types.go to remove/update
	LogBackend      string            `json:"logBackend"`         // 指定的日志后端
	TargetNamespace string            `json:"targetNamespace"`    // 过滤的ns
	Selector        map[string]string `json:"selector,omitempty"` // 过滤pod的标签
	CollectPattern  string            `json:"collectPattern"`     // 配置的采集正则表达式
	//CollectPatternReg *regexp.Regexp        `json:"-" yaml:"-" `        //给内部用的采集正则表达式
	LogAlert *LogAlertObj `json:"logAlert"` // 日志告警的配置
}

type LogAlertObj struct {
	Url            string   `json:"url"`
	KeyWordPattern string   `json:"keyWordPattern"` // 配置的告警正则表达式
	AtMobiles      []string `json:"atMobiles"`      // 钉钉at手机号
	//KeyWordPatternReg *regexp.Regexp `json:"-" yaml:"-" `    //给内部用的告警正则表达式
}

type CollectRulePhase string

const (
	StatusCollectRulePending    CollectRulePhase = "Pending"   // 等待日志采集中
	StatusCollectRuleCollecting CollectRulePhase = "Collected" // 采集到日志了
	StatusCollectRuleDeleting   CollectRulePhase = "Deleting"
)

// CollectRuleStatus defines the observed state of CollectRule
type CollectRuleStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	SyncPhase      CollectRulePhase `json:"syncPhase"`      // 定义同步状态
	LastDeployTime *metav1.Time     `json:"lastDeployTime"` // metav1.Time 定义时间
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.syncPhase`
// +kubebuilder:printcolumn:name="LastDeployTime",type="date",JSONPath=".status.lastDeployTime"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:shortName=cr,scope=Cluster
// CollectRule is the Schema for the collectrules API
type CollectRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CollectRuleSpec   `json:"spec,omitempty"`
	Status CollectRuleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CollectRuleList contains a list of CollectRule
type CollectRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CollectRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CollectRule{}, &CollectRuleList{})
}
