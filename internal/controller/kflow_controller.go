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
	"time"

	"apiserver/api/v1alpha1"
	kflowiov1alpha1 "apiserver/api/v1alpha1"

	"github.com/go-redis/redis/v8"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
			taskName := parts[2]
			var pvList corev1.PersistentVolumeList
			err := r.Client.List(ctx, &pvList, &client.ListOptions{
				Namespace: req.Namespace,
			})
			//for _, pvc := range pvList.Items {
			//	ctrl.Log.Info("PV List",
			//		"name", pvc.Name,
			//		"spec", pvc.Spec)
			//}

			var kflow kflowiov1alpha1.Kflow
			kflowKey := types.NamespacedName{
				Name:      "kflow-sample", // 假设 Kflow 资源的名称为 "kflow-sample"
				Namespace: "default",      // 假设 Kflow 资源的命名空间为 "default"
			}
			err = r.Get(ctx, kflowKey, &kflow)
			if kflow.Status.Tasks == nil {
				log.Info("Status update not ready")
				return reconcile.Result{}, err
			}
			//log.Info("show task status",
			//	"task name", parts[2],
			//	"task status", kflow.Status.Tasks[parts[2]])

			taskStatus := kflow.Status.Tasks[taskName]
			taskStatus.Status = "completed"
			kflow.Status.Tasks[taskName] = taskStatus
			//if err := r.Status().Update(ctx, &kflow); err != nil {
			//	log.Error(err, "Failed to update Kflow status")
			//	return reconcile.Result{}, err
			//}

			for _, next := range kflow.Status.Tasks[taskName].Nexts {
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

		if kflow.Status.Grouped == false {
			log.Info("is kflow")
			ctrl.Log.Info("start group nodes")
			nodeList := &corev1.NodeList{}
			if err := r.List(context.Background(), nodeList); err != nil {
				return reconcile.Result{}, err
			}
			ctrl.Log.Info("show node count", "node count", len(nodeList.Items))

			kflow.Status.Nodes = make([]string, len(nodeList.Items))
			kflow.Status.Tasks = make(map[string]kflowiov1alpha1.TaskStatus)
			kflow.Status.Pv = make(map[string]string)

			for i := 0; i < len(nodeList.Items); i++ {
				selectedNode := nodeList.Items[i].Name
				kflow.Status.Pv[selectedNode] = r.CreatePV(ctx, selectedNode).Name
				kflow.Status.Nodes[i] = selectedNode
			}

			err := r.groupTasks(ctx, &kflow, *nodeList)
			if err != nil {
				log.Error(err, "Failed to group tasks")
				return reconcile.Result{}, err
			}
			//if err := r.Status().Update(ctx, &kflow); err != nil {
			//	log.Error(err, "Failed to update Kflow status")
			//	return reconcile.Result{}, err
			//}

			log.Info("Start schedule task0")
			task := kflow.Spec.Tasks["task1"]
			//for _, group := range kflow.Status.Groups {
			//	ctrl.Log.Info("show groups", "group", group)
			//}
			//ctrl.Log.Info("show task", "task status", kflow.Status.Tasks[task.Name])
			//ctrl.Log.Info("show task pvc", "task pvc", kflow.Status.Tasks[task.Name].Group.Pvc)
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

func (r *KflowReconciler) groupTasks(ctx context.Context, kflow *v1alpha1.Kflow, nodeList corev1.NodeList) error {
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
		//ctrl.Log.Info("start schedule FaasFlow")
		groupedNodes := make(map[string]groupedNode, 0)
		maxTasks := kflow.Spec.GroupPolicy.MaxTasks
		id := 0
		CheckIfGrouped := func(task kflowiov1alpha1.TaskSpec) bool {
			//ctrl.Log.Info("CheckIfGrouped",
			//	"task name", task.Name,
			//	"group id", groupedNodes[task.Name].id)
			if groupedNodes[task.Name].id == 0 {
				//ctrl.Log.Info("Not grouped")
				return false
			}
			//ctrl.Log.Info("Is grouped")
			return true
		}
		CheckIfLessThanLimit := func(sid int, eid int) bool {
			return len(groupedTasks[sid])+len(groupedTasks[eid]) < maxTasks
		}
		Merge := func(sid int, eid int) {
			//ctrl.Log.Info("merge two group",
			//	"start group id", sid,
			//	"end group id", eid)
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
			//ctrl.Log.Info("show current edge",
			//	"start node name", cPath[i].Start.Name,
			//	"start node id", groupedNodes[cPath[i].Start.Name].id,
			//	"end node name", cPath[i].End.Name,
			//	"end node id", groupedNodes[cPath[i].End.Name].id)
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
					//ctrl.Log.Info("add End node into Start group",
					//	"start node", cPath[i].Start.Name,
					//	"start node id", groupedNodes[cPath[i].Start.Name].id,
					//	"end node", cPath[i].End,
					//	"end node id", groupedNodes[cPath[i].End.Name].id)
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
					//ctrl.Log.Info("add Start node into End group",
					//	"start node", cPath[i].Start.Name,
					//	"start node id", groupedNodes[cPath[i].Start.Name].id,
					//	"end node", cPath[i].End,
					//	"end node id", groupedNodes[cPath[i].End.Name].id)
				}
			} else {
				id++
				//ctrl.Log.Info("two tasks not grouped",
				//	"task1", cPath[i].Start.Name,
				//	"task2", cPath[i].End.Name,
				//	"group id", id)
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
			//ctrl.Log.Info("Last check if omit",
			//	"task name", task.Name,
			//	"id", groupedNodes[task.Name].id)
			if groupedNodes[task.Name].id == 0 {
				//ctrl.Log.Info("task not grouped",
				//	"task name", task.Name)
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
			//ctrl.Log.Info("grouped node",
			//	"node name ", node.task.Name,
			//	"group id", node.id)
			groupedTasks[node.id] = append(groupedTasks[node.id], node.task)

		}

	default:
		return nil
	}

	kflow.Status.Groups = r.SetGroupStatus(groupedTasks, "Createing")
	ctrl.Log.Info("show groupTasks")
	for i, group := range kflow.Status.Groups {
		kflow.Status.Groups[i].Node = nodeList.Items[r.SelectNode(i)].Name
		//kflow.Status.Groups[i].Tasks = groupedTasks[i]
		kflow.Status.Groups[i].Pvc = r.CreatePVC(ctx, kflow.Status.Groups[i]).Name
		//ctrl.Log.Info("show pvc", "pvc", kflow.Status.Groups[i].Pvc)
		//ctrl.Log.Info("show group ", "group", kflow.Status.Groups[i])
		for _, task := range group.Tasks {
			kflow.Status.Tasks[task.Name] = r.CreateTaskStatus(kflow.Status.Groups[i], task, kflow.Status.Groups[i].Pvc)
			//ctrl.Log.Info("show task status ", "task status", kflow.Status.Tasks[task.Name])
		}
	}
	kflow.Status.Grouped = true
	//if err := r.Status().Update(ctx, kflow); err != nil {
	//	ctrl.Log.Error(err, "Failed to update Kflow status")
	//}
	return nil
}
func (r *KflowReconciler) CreatePVC(ctx context.Context, grouStatus kflowiov1alpha1.GroupStatus) corev1.PersistentVolumeClaim {
	ctrl.Log.Info("start create pvc")
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("group-%d-pvc", grouStatus.GroupID),
			Namespace: "kflow-worker",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Ki"),
				},
			},
			VolumeName: fmt.Sprintf("node-%s-pv", grouStatus.Node),
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
		},
	}
	err := r.Create(ctx, &pvc)
	if err != nil {
		ctrl.Log.Error(err, "Unable to create PersistentVolumeClaim")
	}
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(2 * time.Second) // 每 2 秒检查一次 PVC 状态
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			ctrl.Log.Error(fmt.Errorf("timeout waiting for PVC to be bound"), "PVC creation timed out")
			return pvc
		case <-ticker.C:
			// 获取 PV 的当前状态
			var pvcStatus corev1.PersistentVolumeClaim
			err := r.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, &pvcStatus)
			if err != nil {
				ctrl.Log.Error(err, "Unable to get PVC status")
				return pvc
			}

			// 检查 PVC 是否已绑定
			if pvcStatus.Status.Phase == corev1.ClaimBound {
				ctrl.Log.Info("PVC successfully created", "PVC", pvc.Name)
				ctrl.Log.Info("show pvc", "pvc", pvc)
				return pvc
			}
			// 如果 PVC 还没有绑定，继续等待
			ctrl.Log.Info("PVC is still in Pending state", "PVC", pvc.Name)
		}
	}
}
func (r *KflowReconciler) CreatePV(ctx context.Context, node string) corev1.PersistentVolume {
	ctrl.Log.Info("start create pv")
	pv := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("node-%s-pv", node),
			Namespace: "kflow-worker",
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("2Mi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchFields: []corev1.NodeSelectorRequirement{
								corev1.NodeSelectorRequirement{
									Key:      "metadata.name",
									Operator: corev1.NodeSelectorOperator("In"),
									Values:   []string{node},
								},
							},
						},
					},
				},
			},
			VolumeMode: new(corev1.PersistentVolumeMode),
			PersistentVolumeSource: v1.PersistentVolumeSource{
				Local: &v1.LocalVolumeSource{
					Path: "/home/docker/disk/kflow-test",
				},
			},
			StorageClassName: "standard",
		},
	}
	pvfs := corev1.PersistentVolumeFilesystem
	pv.Spec.VolumeMode = &pvfs
	err := r.Create(ctx, &pv)
	if err != nil {
		ctrl.Log.Error(err, "Unable to create PersistentVolume")
	}

	//轮循等待PV创建完成
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(2 * time.Second) // 每 2 秒检查一次 PVC 状态
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			ctrl.Log.Error(fmt.Errorf("timeout waiting for PVC to be bound"), "PVC creation timed out")
			return pv
		case <-ticker.C:
			// 获取 PV 的当前状态
			var pvStatus corev1.PersistentVolume
			err := r.Get(ctx, types.NamespacedName{Name: pv.Name, Namespace: pv.Namespace}, &pvStatus)
			if err != nil {
				ctrl.Log.Error(err, "Unable to get PVC status")
				return pv
			}

			// 检查 PVC 是否已绑定
			if pvStatus.Status.Phase == corev1.VolumeAvailable {
				ctrl.Log.Info("PV successfully created", "PV", pv.Name)
				return pv
			}
			// 如果 PVC 还没有绑定，继续等待
			ctrl.Log.Info("PV is still in Pending state", "PV", pv.Name)
		}
	}
}

func (r *KflowReconciler) PullData(kflow kflowiov1alpha1.Kflow, taskSpec kflowiov1alpha1.TaskSpec, taskStatus kflowiov1alpha1.TaskStatus) {
	taskPosition := taskStatus.Node
	remoteTasks := make([]kflowiov1alpha1.TaskSpec, 0)
	localTasks := make([]kflowiov1alpha1.TaskSpec, 0)
	for _, depTask := range taskSpec.Depends {
		if kflow.Status.Tasks[depTask].Node != taskPosition { //dep task 和当前task不在同一节点，需要远程读取
			remoteTasks = append(remoteTasks, kflow.Status.Tasks[depTask].Task)
		} else { //dep task和当前task在同一节点，本地读取
			localTasks = append(localTasks, kflow.Status.Tasks[depTask].Task)
		}
	}
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr:     "192.168.2.149:6379", // Redis 地址
		Password: "",                   // 没有密码
		DB:       0,                    // 默认 DB 0
	})
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		ctrl.Log.Error(err, "无法连接到 Redis")
	}
	fmt.Println("成功连接到 Redis")
	/*for _, remoteTask := range remoteTasks {
		redisKey := fmt.Sprintf("data%s", remoteTask.Name)
		raw, err := rdb.Get(ctx, redisKey).Bytes()
		if err != nil {
			panic(err)
		}

		var data map[string]interface{}
		err = msgpack.Unmarshal(raw, &data)
		if err != nil {
			panic(err)
		}
	}*/
}

func (r *KflowReconciler) PushData(kflow kflowiov1alpha1.Kflow, taskSpec kflowiov1alpha1.TaskSpec, taskStatus kflowiov1alpha1.TaskStatus) {

}

func (r *KflowReconciler) SelectNode(id int) int {
	return id % 3
}
func (r *KflowReconciler) CreateTaskStatus(group kflowiov1alpha1.GroupStatus, task kflowiov1alpha1.TaskSpec, pvcname string) kflowiov1alpha1.TaskStatus {
	ctrl.Log.Info("Start Create task status")
	taskstatus := kflowiov1alpha1.TaskStatus{
		Task:    task,
		Status:  "create",
		Node:    group.Node,
		Depends: task.Depends,
		Nexts:   task.Nexts,
		Group:   group,
		//TaskPVC:     group.Pvc,
		TaskPVCName: pvcname,
	}
	//ctrl.Log.Info("show taskstatus.Group.Pvc", "pvc", taskstatus.Group.Pvc)
	//ctrl.Log.Info("show taskstatus.Pvc", "pvc", taskstatus.TaskPVC)
	//ctrl.Log.Info("show taskstatus.Pvc.Name", "pvc", taskstatus.TaskPVCName)
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
	//ctrl.Log.Info("show tasks pvc", "pvc", taskStatus.TaskPVC)
	//ctrl.Log.Info("show taskstatus.Pvc.name", "pvc name", taskStatus.TaskPVCName)
	// 为每个任务创建 Pod
	if taskStatus.Status == "completed" {
		log.Log.Info("task completed", "task name", taskSpec.Name, "status", taskStatus)
		return nil
	}
	if r.CheckDependsStatus(*kflow, taskStatus.Task) {
		ctrl.Log.Info("start build pod", "task name", taskSpec.Name)
		r.PullData(*kflow, taskSpec, taskStatus)
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
				Volumes: []corev1.Volume{
					{
						Name: taskSpec.Name,
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: taskStatus.TaskPVCName,
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:    taskSpec.Name,
						Image:   taskSpec.Image,
						Command: taskSpec.Command,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      taskSpec.Name,
								ReadOnly:  false,
								MountPath: "/mnt/test",
							},
						},
					},
				},
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
