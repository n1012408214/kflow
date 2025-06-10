import json
import yaml
from flask import Flask, request
import uuid
from kubernetes import client, config
from kubernetes.client.rest import ApiException
from kubernetes.dynamic import DynamicClient
app = Flask(__name__)

# 定义结构体
class Request:
    def __init__(self, requestID, tasks, group_policy):
        self.requestID = requestID
        self.tasks = tasks
        self.group_policy = group_policy

class GroupPolicy:
    def __init__(self, type, maxTasks):
        self.type = type
        self.maxTasks = maxTasks

class Task:
    def __init__(self, name, image, command, args, exectime, depends, nexts, input_files_names, output_files_names):
        self.name = name
        self.image = image
        self.command = command
        self.args = args
        self.exectime = exectime
        self.depends = depends
        self.nexts = nexts
        self.input_files_names = input_files_names
        self.output_files_names = output_files_names

@app.route('/run', methods=['POST'])
def run():
    # 获取请求的 JSON 数据
    req = request.get_json(force=True, silent=True)
    
    # 提取并构造 group_policy 对象
    group_policy = GroupPolicy(req['spec']['groupPolicy']['type'], req['spec']['groupPolicy']['maxTasks'])
    request_id = str(uuid.uuid4())
    
    # 提取并构造 task 对象
    tasks = {}
    for task_name, task_data in req['spec']['tasks'].items():
        task_obj = Task(
            task_name,
            task_data.get('image'),
            task_data.get('command'),
            task_data.get('args'),
            task_data.get('execTime'),
            task_data.get('depends', []),
            task_data.get('nexts', []),
            task_data.get('inputfilename', []),
            task_data.get('outputfilename', [])
        )
        tasks[task_name] = task_obj
    
    # 构造 Request 对象
    request_obj = Request(request_id, tasks, group_policy)
    # 将 Request 对象转换为字典格式
    request_dict = {
        'apiVersion': 'kflow.io.kflow/v1alpha1',
        'kind': 'Kflow',
        'metadata': {
            'labels': {
                'app.kubernetes.io/name': 'kflow',
                'app.kubernetes.io/managed-by': 'kustomize'
            },
            'name': 'kflow-sample'+'-'+request_id,
            'namespace': 'default'
        },
        'spec': {
            'requestID': request_obj.requestID,
            'groupPolicy': {
                'type': request_obj.group_policy.type,
                'maxTasks': request_obj.group_policy.maxTasks
            },
            'tasks': {}
        }
    }

    # 将 tasks 添加到字典中
    for task_name, task_obj in request_obj.tasks.items():
        request_dict['spec']['tasks'][task_name] = {
            'name': task_obj.name ,
            'image': task_obj.image,
            'command': task_obj.command,
            'args': task_obj.args,
            'execTime': task_obj.exectime,
            'depends': task_obj.depends,
            'nexts': task_obj.nexts,
            'inputfilename': task_obj.input_files_names,
            'outputfilename': task_obj.output_files_names
        }

    # 将字典转换为 YAML 格式
    k8s_manifest = yaml.dump(request_dict, default_flow_style=False)
    manifest_dict = yaml.safe_load(k8s_manifest)
    # 返回生成的 YAML 文件作为响应
    k8s_client = config.new_client_from_config("/home/njl/.kube/config")

# 创建一个 DynamicClient 实例，用于管理 Kubernetes 资源
    dyn_client = DynamicClient(k8s_client)
    def create_k8s_resource(manifest):
        try:
            # 获取 Kubernetes API 动态对象
            api = dyn_client.resources.get(api_version='kflow.io.kflow/v1alpha1', kind='Kflow')

            # 创建资源
            api.create(body=manifest)
            print("Kubernetes resource created successfully!")

        except ApiException as e:
            print(f"Error when creating Kubernetes resource: {e}")
    create_k8s_resource(manifest_dict)

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5001,threaded=True)
