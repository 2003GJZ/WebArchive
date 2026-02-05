# WebArchive

一个单用户的网页内容归档工具：浏览器插件一键抓取，后端自动清洗与资源本地化，前端管理与预览。

## 目录结构
- `backend/` Go + Gin 后端服务
- `frontend/` Vite + React 管理台
- `extension/` Edge/Chrome MV3 插件
- `deploy/` MySQL + MinIO 的 docker-compose

## 启动依赖
```bash
cd deploy
docker compose up -d
```

## 启动后端
```bash
cd backend
cp .env.example .env
# 根据需要修改 .env

go mod tidy
go run ./cmd/server
```

后端默认地址：`http://localhost:8080`

## 启动前端
```bash
cd frontend
npm install
npm run dev
```

前端默认地址：`http://localhost:5173`

## 安装插件（Edge）
1. 打开 `edge://extensions/`
2. 打开“开发者模式”
3. 点击“加载解压缩的扩展”，选择 `extension/` 目录
4. 打开任意网页，点击插件“WebArchive 采集” → “一键抓取”

> 如果后端地址不是 `http://localhost:8080`，请在 `extension/manifest.json` 的 `host_permissions` 中添加对应域名后重新加载插件。

## 图谱视图
前端提供“知识星球”3D 图谱视图，可基于分类、标签、层级结构进行交互。

## API 简要
- `POST /api/archives` 保存归档
- `GET /api/archives` 列表（支持 `q`、`category`、`tag` 查询）
- `GET /api/archives/:id` 详情
- `PATCH /api/archives/:id` 更新分类/标签
- `DELETE /api/archives/:id` 删除归档
- `POST /api/archives/:id/ai-tag` 使用 LLM 生成分类/标签/层级
- `POST /api/ai/config` 更新 LLM 配置
- `GET /api/taxonomy` 获取分类树
- `GET /api/taxonomy/:id` 获取节点详情（含子类与相关文章）
- `GET /api/graph` 获取知识图谱数据
- `GET /api/archives/:id/html` 归档 HTML
- `GET /api/assets/:id/*path` 资源代理

## LLM 配置
后端支持标准 ChatGPT 格式接口，配置以下环境变量：
```
LLM_BASE_URL=https://api.openai.com/v1
LLM_API_KEY=your_key
LLM_MODEL=gpt-4o-mini
LLM_ENABLED=true
AUTO_TAG_ON_CAPTURE=false
```
