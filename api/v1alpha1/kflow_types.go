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

// KflowSpec defines the desired state of Kflow.
type KflowSpec struct {
	// 任务定义
	RequestID string              `json:"requestID,omitempty"`
	Tasks     map[string]TaskSpec `json:"tasks"`

	// 分组策略（新增字段）
	GroupPolicy GroupPolicy `json:"groupPolicy"`
}

type GroupPolicy struct {
	Type     string `json:"type"`     // "Level" | "DataAffinity" | "Manual"
	MaxTasks int    `json:"maxTasks"` // 每组最大任务数
}

type TaskSpec struct {
	Name    string   `json:"name"`
	Image   string   `json:"image"`
	Command []string `json:"command,omitempty"`

	// +optional
	Args     []string `json:"args,omitempty"`
	ExecTime int      `json:"execTime,omitempty"`

	// +optional
	Depends []string `json:"depends,omitempty"` //
	Nexts   []string `json:"nexts,omitempty"`

	InputFileName  []string `json:"inputfilename,omitempty"`
	OutputFileName string   `json:"outputfilename,omitempty"`
}

type KflowStatus struct {
	Phase string `json:"phase"` // Processing | Completed | Failed

	// 记录每个组的状态
	Grouped bool                  `json:"grouped,omitempty"`
	Groups  []GroupStatus         `json:"groups,omitempty"`
	Tasks   map[string]TaskStatus `json:"tasks,omitempty"`
	Nodes   []string              `json:"groupNodes,omitempty"`
	Pv      map[string]string     `json:"pv,omitempty"`
}

type GroupStatus struct {
	GroupID        int             `json:"groupId"`
	Tasks          []TaskSpec      `json:"tasks"`  // 任务列表
	Status         string          `json:"status"` // Pending | Running | Succeeded | Failed
	GroupStartTime *metav1.Time    `json:"startTime,omitempty"`
	Node           string          `json:"node"`
	Pvc            string          `json:"pvc,omitempty"`
	PulledFiles    map[string]bool `json:"pulledfiles"`
}

type TaskStatus struct {
	Task        TaskSpec    `json:"task"`
	Pod         string      `json:"pod,omitempty"`
	Depends     []string    `json:"depends,omitempty"`
	Nexts       []string    `json:"nexts,omitempty"`
	Group       GroupStatus `json:"tasksGroup,omitempty"`
	Node        string      `json:"node"`
	Status      string      `json:"status"`
	TaskPVCName string      `json:"taskPvcName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Kflow is the Schema for the kflows API.
type Kflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KflowSpec   `json:"spec,omitempty"`
	Status KflowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KflowList contains a list of Kflow.
type KflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Kflow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kflow{}, &KflowList{})
}
