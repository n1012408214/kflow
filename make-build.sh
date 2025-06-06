kubectl delete deployment kflow-controller-manager -n kflow-system
make docker-build
docker tag $(docker image ls --format "{{.Repository}} {{.Tag}} {{.ID}} {{.CreatedSince}}" | grep controller | grep latest | head -n 1 | awk '{print $3}'
) crpi-2rclh8j1lqwo45m4.cn-qingdao.personal.cr.aliyuncs.com/mnikube/controller
docker push crpi-2rclh8j1lqwo45m4.cn-qingdao.personal.cr.aliyuncs.com/mnikube/controller
kubectl delete pod --all -n kflow-worker
kubectl delete crd --all
kubectl delete pv --all -n kflow-worker
kubectl delete pvc --all -n kflow-worker
kubectl apply -f ./config/crd/bases/kflow.io.kflow_kflows.yaml 
kubectl apply -f /home/njl/kflow/config/samples/kflow.io_v1alpha1_kflow.yaml
make deploy
#kubectl delete pod $(kubectl get pods -n kflow-system | grep kflow-controller | awk '{print $1}') -n kflow-system