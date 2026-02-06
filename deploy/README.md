# Docker Compose 部署说明

## 开发环境

仅启动 MySQL 和 MinIO，前后端在本地运行：

```bash
docker-compose -f docker-compose.dev.yml up -d
```

停止服务：

```bash
docker-compose -f docker-compose.dev.yml down
```

## 生产环境

一键启动所有服务（MySQL、MinIO、后端、前端）：

```bash
docker-compose up -d
```

停止所有服务：

```bash
docker-compose down
```

查看日志：

```bash
docker-compose logs -f
```

## 访问地址

- 前端：http://localhost:5173
- 后端 API：http://localhost:8080
- MinIO 控制台：http://localhost:9001
- MySQL：localhost:3306

## 环境变量配置

如需修改后端环境变量，请编辑 `docker-compose.yml` 中 backend 服务的 environment 部分。
