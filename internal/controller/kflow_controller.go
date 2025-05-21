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

package controller

import (
	// 标准库
	"context"
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strings"

	"apiserver/api/v1alpha1"
	kflowiov1alpha1 "apiserver/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// KflowReconciler reconciles a Kflow object

// KflowReconciler reconciles a Kflow object
type KflowReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile 执行任务分配和调度
func (r *KflowReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx)
	//log.Info("Reconciling Kflow111", "name", req.Name)

	var kflow v1alpha1.Kflow
	err_kflow := r.Get(ctx, req.NamespacedName, &kflow)
	var pod corev1.Pod
	err_pod := r.Get(ctx, req.NamespacedName, &pod)
	/*ctrl.Log.Info("show reconcile Request",
	"name", req.Name,
	"namespace", req.Namespace,
	"namespacename", req.NamespacedName)*/

	//pod triger
	if err_pod == nil && err_kflow != nil {

		if !strings.HasPrefix(pod.Name, "kflow-sample-") {
			return reconcile.Result{}, client.IgnoreNotFound(err_kflow)
		} else {
			log.Info("is task pod",
				"pod name", pod.Name)
			parts := strings.Split(pod.Name, "-")

			var kflow kflowiov1alpha1.Kflow
			kflowKey := types.NamespacedName{
				Name:      "kflow-sample", // 假设 Kflow 资源的名称为 "kflow-sample"
				Namespace: "default",      // 假设 Kflow 资源的命名空间为 "default"
			}
			err := r.Get(ctx, kflowKey, &kflow)
			if kflow.Status.Tasks == nil {
				log.Info("Status update not ready")
				return reconcile.Result{}, err
			}
			log.Info("show task status",
				"task name", parts[2],
				"task status", kflow.Status.Tasks[parts[2]])

			taskStautus := kflow.Status.Tasks[parts[2]]
			taskStautus.Status = "completed"
			kflow.Status.Tasks[parts[2]] = taskStautus
			if err := r.Status().Update(ctx, &kflow); err != nil {
				log.Error(err, "Failed to update Kflow status")
				return reconcile.Result{}, err
			}

			for _, next := range taskStautus.Nexts {
				log.Info("start schedule task", "task name", next)
				err = r.scheduleTasks(&kflow, kflow.Spec.Tasks[next], kflow.Status.Tasks[next])
				if err != nil {
					log.Error(err, "Failed to schedule tasks")
					return reconcile.Result{}, err
				}
			}

		}

	}
	//kflow triger
	if err_kflow == nil && err_pod != nil {
		log.Info("is kflow")

		if kflow.Status.Grouped == false {
			ctrl.Log.Info("start group nodes")
			nodeList := &corev1.NodeList{}
			if err := r.List(context.Background(), nodeList); err != nil {
				return reconcile.Result{}, err
			}
			groupCount := len(nodeList.Items)
			ctrl.Log.Info("show node count", "node count", groupCount)

			kflow.Status.GroupNodes = make([]string, groupCount)
			kflow.Status.Tasks = make(map[string]kflowiov1alpha1.TaskStatus)

			for i := 0; i < groupCount; i++ {
				selectedNode := nodeList.Items[i].Name
				kflow.Status.GroupNodes[i] = selectedNode
			}

			err := r.groupTasks(&kflow)
			if err != nil {
				log.Error(err, "Failed to group tasks")
				return reconcile.Result{}, err
			}
			log.Info("Start schedule task0")
			task := kflow.Spec.Tasks["task1"]
			err = r.scheduleTasks(&kflow, task, kflow.Status.Tasks[task.Name])
			if err != nil {
				log.Error(err, "Failed to schedule tasks")
				return reconcile.Result{}, err
			}

		}
		if err := r.Status().Update(ctx, &kflow); err != nil {
			log.Error(err, "Failed to update Kflow status")
			return reconcile.Result{}, err
		}
	}
	//not kflow or pod
	if err_kflow != nil && err_pod != nil {
		log.Info("not kflow or pod", "request name", req.Name)
		return reconcile.Result{}, client.IgnoreNotFound(err_kflow)
	}

	return reconcile.Result{}, nil
}

func (r *KflowReconciler) groupTasks(kflow *v1alpha1.Kflow) error {
	ctrl.Log.Info("Start group Tasks")
	groupedTasks := make(map[int][]kflowiov1alpha1.TaskSpec)
	kflow.Status.Groups = r.SetGroupStatus(groupedTasks, "pending")
	groupPolicy := kflow.Spec.GroupPolicy

	switch groupPolicy.Type {
	/*case "Level":
		for i, task := range kflow.Spec.Tasks {
			groupID := i / groupPolicy.MaxTasks
			groupedTasks[groupID] = append(groupedTasks[groupID], task)
			kflow.Status.Tasks[task.Name] = r.CreateTaskStatus(groupID, task, kflow)
			ctrl.Log.Info("show created status",
				"task ", task.Name,
				"status", kflow.Status.Tasks[task.Name])
		}
	case "DataAffinity":
		for _, task := range kflow.Spec.Tasks {
			groupID := hashDataPath(task.InputPath) % groupPolicy.MaxTasks
			groupedTasks[groupID] = append(groupedTasks[groupID], task)
			kflow.Status.Tasks[task.Name] = r.CreateTaskStatus(groupID, task, kflow)
		}
	case "Manual":
		for i, task := range kflow.Spec.Tasks {
			groupID := i / groupPolicy.MaxTasks
			groupedTasks[groupID] = append(groupedTasks[groupID], task)
			kflow.Status.Tasks[task.Name] = r.CreateTaskStatus(groupID, task, kflow)
		}*/
	case "FaasFLOW":
		Tasks := kflow.Spec.Tasks
		type cPathEdge struct {
			Start    kflowiov1alpha1.TaskSpec
			End      kflowiov1alpha1.TaskSpec
			execTime int
		}
		type groupedNode struct {
			task kflowiov1alpha1.TaskSpec
			id   int
		}
		ctrl.Log.Info("start schedule FaasFlow")
		groupedNodes := make(map[string]groupedNode, 0)
		maxTasks := kflow.Spec.GroupPolicy.MaxTasks
		id := 0
		CheckIfGrouped := func(task kflowiov1alpha1.TaskSpec) bool {
			ctrl.Log.Info("CheckIfGrouped",
				"task name", task.Name,
				"group id", groupedNodes[task.Name].id)
			if groupedNodes[task.Name].id == 0 {
				ctrl.Log.Info("Not grouped")
				return false
			}
			ctrl.Log.Info("Is grouped")
			return true
		}
		CheckIfLessThanLimit := func(sid int, eid int) bool {
			return len(groupedTasks[sid])+len(groupedTasks[eid]) < maxTasks
		}
		Merge := func(sid int, eid int) {
			ctrl.Log.Info("merge two group",
				"start group id", sid,
				"end group id", eid)
			groupedTasks[sid] = append(groupedTasks[sid], groupedTasks[eid]...)
			for _, task := range groupedTasks[eid] {
				newGroupedNode := groupedNode{
					task: groupedNodes[task.Name].task,
					id:   eid,
				}
				groupedNodes[task.Name] = newGroupedNode
			}
			delete(groupedTasks, eid)
		}
		cPath := make([]cPathEdge, 0)
		cPathCout := 0

		for _, task := range Tasks {
			for _, ntask := range task.Nexts {
				cPathCout++
				edge := cPathEdge{
					Start:    task,
					End:      kflow.Spec.Tasks[ntask],
					execTime: task.ExecTime,
				}
				//ctrl.Log.Info("build edge",
				//	"start", task.Name,
				//	"end", ntask,
				//	"exectime", task.ExecTime)
				cPath = append(cPath, edge)
			}
		}
		sort.Slice(cPath, func(i, j int) bool {
			return cPath[i].execTime > cPath[j].execTime
		})
		//for _, edge := range cPath {
		//	ctrl.Log.Info("exec ",
		//		"time", edge.execTime)
		//}

		for i := 0; i < cPathCout; i++ {
			ctrl.Log.Info("show current edge",
				"start node name", cPath[i].Start.Name,
				"start node id", groupedNodes[cPath[i].Start.Name].id,
				"end node name", cPath[i].End.Name,
				"end node id", groupedNodes[cPath[i].End.Name].id)
			if CheckIfGrouped(cPath[i].Start) && CheckIfGrouped(cPath[i].End) {
				sid := groupedNodes[cPath[i].Start.Name].id
				eid := groupedNodes[cPath[i].End.Name].id
				if CheckIfLessThanLimit(sid, eid) {
					Merge(sid, eid)
				}
			} else if CheckIfGrouped(cPath[i].Start) {
				curid := groupedNodes[cPath[i].Start.Name].id
				if len(groupedTasks[curid]) < maxTasks {
					groupedTasks[curid] = append(groupedTasks[curid], cPath[i].End)
					NewNode := groupedNodes[cPath[i].End.Name]
					NewNode = groupedNode{
						id:   curid,
						task: cPath[i].End,
					}
					groupedNodes[cPath[i].End.Name] = NewNode
					ctrl.Log.Info("add End node into Start group",
						"start node", cPath[i].Start.Name,
						"start node id", groupedNodes[cPath[i].Start.Name].id,
						"end node", cPath[i].End,
						"end node id", groupedNodes[cPath[i].End.Name].id)
				}

			} else if CheckIfGrouped(cPath[i].End) {
				curid := groupedNodes[cPath[i].End.Name].id
				if len(groupedTasks[curid]) < maxTasks {
					groupedTasks[curid] = append(groupedTasks[curid], cPath[i].Start)
					NewNode := groupedNodes[cPath[i].Start.Name]
					NewNode = groupedNode{
						id:   curid,
						task: cPath[i].Start,
					}
					groupedNodes[cPath[i].Start.Name] = NewNode
					ctrl.Log.Info("add Start node into End group",
						"start node", cPath[i].Start.Name,
						"start node id", groupedNodes[cPath[i].Start.Name].id,
						"end node", cPath[i].End,
						"end node id", groupedNodes[cPath[i].End.Name].id)
				}
			} else {
				id++
				ctrl.Log.Info("two tasks not grouped",
					"task1", cPath[i].Start.Name,
					"task2", cPath[i].End.Name,
					"group id", id)
				groupedTasks[id] = append(groupedTasks[id], cPath[i].Start)
				groupedTasks[id] = append(groupedTasks[id], cPath[i].End)
				groupedNodes[cPath[i].Start.Name] = groupedNode{
					task: cPath[i].Start,
					id:   id,
				}
				groupedNodes[cPath[i].End.Name] = groupedNode{
					task: cPath[i].End,
					id:   id,
				}
			}
		}
		for _, task := range kflow.Spec.Tasks {
			ctrl.Log.Info("Last check if omit",
				"task name", task.Name,
				"id", groupedNodes[task.Name].id)
			if groupedNodes[task.Name].id == 0 {
				ctrl.Log.Info("task not grouped",
					"task name", task.Name)
				id++
				OmitTask := groupedNode{
					id:   id,
					task: task,
				}
				groupedNodes[OmitTask.task.Name] = OmitTask
			}
		}
		groupedTasks = make(map[int][]kflowiov1alpha1.TaskSpec)
		for _, node := range groupedNodes {
			ctrl.Log.Info("grouped node",
				"node name ", node.task.Name,
				"group id", node.id)
			groupedTasks[node.id] = append(groupedTasks[node.id], node.task)
			kflow.Status.Tasks[node.task.Name] = r.CreateTaskStatus((node.id)%3, node.task, kflow)
		}

	default:
		return nil
	}

	kflow.Status.Groups = r.SetGroupStatus(groupedTasks, "runnning")
	kflow.Status.Grouped = true
	return nil
}

func (r *KflowReconciler) CreateTaskStatus(groupID int, task kflowiov1alpha1.TaskSpec, kflow *kflowiov1alpha1.Kflow) kflowiov1alpha1.TaskStatus {
	taskstatus := kflowiov1alpha1.TaskStatus{
		Task:    task,
		Status:  "create",
		Node:    kflow.Status.GroupNodes[groupID],
		Depends: task.Depends,
		Nexts:   task.Nexts,
	}
	return taskstatus
}

func hashDataPath(s string) int {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int(h.Sum32())
}

// scheduleTasks 为每个任务组选择一个节点，并为任务创建 Pod
func (r *KflowReconciler) scheduleTasks(kflow *v1alpha1.Kflow, taskSpec kflowiov1alpha1.TaskSpec, taskStatus kflowiov1alpha1.TaskStatus) error {
	ctrl.Log.Info("Start schedule Tasks")

	// 为每个任务创建 Pod
	if taskStatus.Status == "completed" {
		log.Log.Info("task completed", "task name", taskSpec.Name, "status", taskStatus)
		return nil
	}
	if r.CheckDependsStatus(*kflow, taskStatus.Task) {
		ctrl.Log.Info("start build pod", "task name", taskSpec.Name)
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", kflow.Name, taskSpec.Name),
				Namespace: "kflow-worker",
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				NodeSelector: map[string]string{
					"kubernetes.io/hostname": taskStatus.Node,
				},
				Containers: []corev1.Container{
					{
						Name:    taskSpec.Name,
						Image:   taskSpec.Image,
						Command: taskSpec.Command,
					},
				},
				//Affinity: &corev1.Affinity{
				//	NodeAffinity: &corev1.NodeAffinity{
				//		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				//			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				//				{
				//					MatchExpressions: []corev1.NodeSelectorRequirement{
				//						{
				//							Key:      "kubernetes.io/hostname",
				//							Operator: corev1.NodeSelectorOpIn,
				//							Values:   []string{nodeName},
				//						},
				//					},
				//				},
				//			},
				//		},
				//	},
				//},
			},
		}

		ctrl.Log.Info("start create contrainer")
		if err := r.Create(context.Background(), pod); err != nil {
			return err
		}
		taskStatus := kflow.Status.Tasks[taskSpec.Name]
		taskStatus.Pod = pod.Name
		taskStatus.Status = "Running"
		kflow.Status.Tasks[taskSpec.Name] = taskStatus
	}

	return nil
}

func (r *KflowReconciler) CheckDependsStatus(kflow kflowiov1alpha1.Kflow, TaskSpec kflowiov1alpha1.TaskSpec) bool {
	log.Log.Info("Check Depends Status",
		"current task", TaskSpec.Name)
	depends := kflow.Status.Tasks[TaskSpec.Name].Depends
	for _, task := range depends {
		log.Log.Info("check Depend",
			"depend name", task,
			"status", kflow.Status.Tasks[task].Status)
		if kflow.Status.Tasks[task].Status != "completed" {
			return false
		}
	}
	return true
}

// createGroupStatus 生成每个组的状态
func (r *KflowReconciler) SetGroupStatus(groupedTasks map[int][]kflowiov1alpha1.TaskSpec, status string) []v1alpha1.GroupStatus {
	var groups []v1alpha1.GroupStatus
	for groupID, tasks := range groupedTasks {
		group := v1alpha1.GroupStatus{
			GroupID: groupID,
			Tasks:   tasks,
			Status:  status,
		}
		groups = append(groups, group)
	}
	return groups
}

// SetupWithManager sets up the controller with the Manager.
func (r *KflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// 设置 Kflow 资源的 Watch
	err := ctrl.NewControllerManagedBy(mgr).
		For(&kflowiov1alpha1.Kflow{}).
		Complete(r)
	if err != nil {
		return err
	}

	// 设置 Pod 资源的 Watch，并使用事件过滤器
	err = ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				pod := e.ObjectNew.(*corev1.Pod)

				// 使用正则表达式匹配 Pod 名称是否以 "kflow-sample-" 为前缀
				matched, _ := regexp.MatchString("kflow-sample-", pod.GetName())
				if !matched {
					return false // 只处理名称以 "kflow-sample-" 开头的 Pod
				}

				// 其他过滤条件：Pod 的状态必须是 Succeeded 或 Failed，且命名空间是 "kflow-worker"
				return (pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed) &&
					pod.GetNamespace() == "kflow-worker"
			},
		}).
		Complete(r)
	return err
}
