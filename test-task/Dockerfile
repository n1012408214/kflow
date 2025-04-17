# 使用官方的 Python 运行时作为基础镜像
FROM python:3.10-slim

# 设置工作目录
WORKDIR /app

# 复制当前目录下所有文件到容器中
COPY . .

# 设置默认启动命令
CMD ["python", "test.py"]
