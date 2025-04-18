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

	if err_pod == nil && err_kflow != nil {
		//log.Info("is pod")

		if !strings.HasPrefix(pod.Name, "kflow-sample-") {
			//log.Info("pod name prefix uncorrect")
			return reconcile.Result{}, client.IgnoreNotFound(err_kflow)
		} else {
			log.Info("is task pod",
				"pod name", pod.Name)
			parts := strings.Split(pod.Name, "-")
			//time.Sleep(3 * time.Second)

			var kflow kflowiov1alpha1.Kflow
			kflowKey := types.NamespacedName{
				Name:      "kflow-sample", // 假设 Kflow 资源的名称为 "kflow-sample"
				Namespace: "default",      // 假设 Kflow 资源的命名空间为 "default"
			}
			err := r.Get(ctx, kflowKey, &kflow)
			if err != nil {
				log.Error(err_kflow, "failed to get Kflow resource")
				return reconcile.Result{}, err_kflow
			}

			if kflow.Status.Tasks == nil {
				log.Info("Status update not ready")
				return reconcile.Result{}, err
			}
			// 成功获取 Kflow 资源后，你可以在这里继续处理
			//log.Info("checking task status", "task name", parts[2])
			//log.Info("show task status", "status", kflow.Status.Tasks)
			//log.Info("show status", "status", kflow.Status)
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

			for _, Group := range kflow.Status.Groups {
				err := r.scheduleTasks(&kflow, Group.Tasks)
				if err != nil {
					log.Error(err, "Failed to schedule tasks")
					return reconcile.Result{}, err
				}
			}

		}

	}

	if err_kflow == nil && err_pod != nil {
		log.Info("is kflow")
		if kflow.Status.Grouped == false {
			// Step 1: 分配分组node
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

			//Step3：任务分组
			err := r.groupTasks(&kflow)
			if err != nil {
				log.Error(err, "Failed to group tasks")
				return reconcile.Result{}, err
			}

			for _, Group := range kflow.Status.Groups {
				err := r.scheduleTasks(&kflow, Group.Tasks)
				if err != nil {
					log.Error(err, "Failed to schedule tasks")
					return reconcile.Result{}, err
				}
			}
		}
		if err := r.Status().Update(ctx, &kflow); err != nil {
			log.Error(err, "Failed to update Kflow status")
			return reconcile.Result{}, err
		}
	}

	if err_kflow != nil && err_pod != nil {
		log.Info("not kflow or pod", "request name", req.Name)
		return reconcile.Result{}, client.IgnoreNotFound(err_kflow)
	}

	return reconcile.Result{}, nil
}

// groupTasks 将任务分组
func (r *KflowReconciler) groupTasks(kflow *v1alpha1.Kflow) error {
	ctrl.Log.Info("Start group Tasks")
	groupedTasks := make(map[int][]kflowiov1alpha1.TaskSpec)
	kflow.Status.Groups = r.SetGroupStatus(groupedTasks, "pending")
	groupPolicy := kflow.Spec.GroupPolicy

	// 按照分组策略进行分组
	switch groupPolicy.Type {
	case "Level":
		// Level 类型：轮询分组
		for i, task := range kflow.Spec.Tasks {
			groupID := i / groupPolicy.MaxTasks
			groupedTasks[groupID] = append(groupedTasks[groupID], task)
			kflow.Status.Tasks[task.Name] = r.CreateTaskStatus(groupID, task, kflow)
			ctrl.Log.Info("show created status",
				"task ", task.Name,
				"status", kflow.Status.Tasks[task.Name])
		}
	case "DataAffinity":
		// DataAffinity 类型：按数据路径分组（假设这里是基于 InputPath 或 OutputPath）
		for _, task := range kflow.Spec.Tasks {
			groupID := hashDataPath(task.InputPath) % groupPolicy.MaxTasks
			groupedTasks[groupID] = append(groupedTasks[groupID], task)
			kflow.Status.Tasks[task.Name] = r.CreateTaskStatus(groupID, task, kflow)
		}
	case "Manual":
		// Manual 类型：由用户自定义，可以用元数据来分配任务到组
		// 示例：按任务名称进行分组（假设用户已经在任务描述里提供了分组策略）
		for i, task := range kflow.Spec.Tasks {
			groupID := i / groupPolicy.MaxTasks
			groupedTasks[groupID] = append(groupedTasks[groupID], task)
			kflow.Status.Tasks[task.Name] = r.CreateTaskStatus(groupID, task, kflow)
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
	}
	return taskstatus
}

func hashDataPath(s string) int {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int(h.Sum32())
}

// scheduleTasks 为每个任务组选择一个节点，并为任务创建 Pod
func (r *KflowReconciler) scheduleTasks(kflow *v1alpha1.Kflow, Tasks []kflowiov1alpha1.TaskSpec) error {
	ctrl.Log.Info("Start schedule Tasks")

	// 为每个任务创建 Pod
	for _, TaskSpec := range Tasks {

		if kflow.Status.Tasks[TaskSpec.Name].Status == "completed" {
			log.Log.Info("task completed", "task name", TaskSpec.Name, "status", kflow.Status.Tasks[TaskSpec.Name])
			continue
		}
		if r.CheckDependsStatus(*kflow, TaskSpec) {
			ctrl.Log.Info("start build pod", "task name", TaskSpec.Name)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s", kflow.Name, TaskSpec.Name),
					Namespace: "kflow-worker",
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": kflow.Status.Tasks[TaskSpec.Name].Node, // 替换为目标节点的名称
					},
					Containers: []corev1.Container{
						{
							Name:    TaskSpec.Name,
							Image:   TaskSpec.Image, // 需要替换为实际的任务镜像
							Command: TaskSpec.Command,
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
			taskStatus := kflow.Status.Tasks[TaskSpec.Name]
			taskStatus.Pod = pod.Name
			taskStatus.Status = "Running"
			kflow.Status.Tasks[TaskSpec.Name] = taskStatus
		}

	}
	return nil
}

func (r *KflowReconciler) CheckDependsStatus(kflow kflowiov1alpha1.Kflow, TaskSpec kflowiov1alpha1.TaskSpec) bool {
	log.Log.Info("Check Depends Status",
		"current task", TaskSpec.Name)
	depends := kflow.Status.Tasks[TaskSpec.Name].Depends
	for _, taskName := range depends {
		log.Log.Info("check Depend",
			"depend name", taskName,
			"status", kflow.Status.Tasks[taskName].Status)
		if kflow.Status.Tasks[taskName].Status != "completed" {
			return false
		}
	}
	return true
}

//func (r *KflowReconciler) SetTaskStatus(kflow kflowiov1alpha1.Kflow, pod corev1.Pod) error {
//
//}

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
				matched, _ := regexp.MatchString("^kflow-sample-", pod.GetName())
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

/*func (r *KflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kflowiov1alpha1.Kflow{}).
		// 监听 Pod 资源
		Watches(
			builder.Kind(mgr.GetScheme(), &corev1.Pod{}),
			handler.EnqueueRequestsFromMapFunc(handler.MapFunc(func(obj client.Object) []reconcile.Request {
				pod, ok := obj.(*corev1.Pod)
				if !ok {
					return nil
				}

				if !strings.HasPrefix(pod.GetName(), "kflow-sample-") {
					return nil
				}

				if pod.GetNamespace() != "kflow-worker" ||
					(pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed) {
					return nil
				}

				kflowName := pod.Labels["kflow-owner"]
				if kflowName == "" {
					return nil
				}

				return []reconcile.Request{
					{
						NamespacedName: client.ObjectKey{
							Name:      kflowName,
							Namespace: pod.Namespace,
						},
					},
				}
			})),
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					newPod := e.ObjectNew.(*corev1.Pod)
					return newPod.Status.Phase == corev1.PodSucceeded || newPod.Status.Phase == corev1.PodFailed
				},
			}),
		).
		Complete(r)
}*/
