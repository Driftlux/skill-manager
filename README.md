# Codex Skill Manager

本地运行的 Codex 技能和插件管理平台，用来查看、启用、禁用和软删除本机 Codex 用户技能，并查看、启用、禁用 Codex 插件。

## 功能

- 查看 Codex skills，按插件、用户自装、系统技能分组。
- 通过 `/Users/spc/.codex/config.toml` 启用或禁用技能，不移动技能目录。
- 查看插件列表和插件带来的 skills。
- 通过 `[plugins."<id>"]` 修改插件启用状态。
- 用户自装技能支持软删除到 `/Users/spc/.codex/skills.trash/`。
- 兼容历史目录 `/Users/spc/.codex/skills.disabled/`，支持迁移回用户技能目录并保持禁用状态。
- 修改 `config.toml` 前自动创建备份文件。

## 安全边界

应用只允许管理以下路径：

- 用户技能：`/Users/spc/.codex/skills/`
- 系统技能只读：`/Users/spc/.codex/skills/.system/`
- 插件缓存只读：`/Users/spc/.codex/plugins/cache/`
- Codex 配置：`/Users/spc/.codex/config.toml`

禁止删除或修改：

- `/Users/spc/.codex/skills/.system/`
- `/Users/spc/.codex/plugins/cache/`
- `/Users/spc/.codex/memories/`

技能启用/禁用只修改 `[[skills.config]]`。插件启用/禁用只修改目标 `[plugins."<id>"]` 块。删除用户技能时不会永久删除目录，而是移动到 `skills.trash`。

## 技术栈

- Go HTTP API
- Vite
- React
- TypeScript

## 本地开发

安装前端依赖：

```bash
npm install
```

构建前端：

```bash
npm run build
```

运行后端：

```bash
go run ./cmd/skill-manager -addr 127.0.0.1:5174 -static-dir web/dist
```

访问：

```text
http://127.0.0.1:5174
```

## API

### Skills

```http
GET /api/skills
POST /api/skills/:name/enable
POST /api/skills/:name/disable
POST /api/skills/:name/migrate-legacy-disabled
DELETE /api/skills/:name
```

`GET /api/skills` 返回用户、自带系统、插件技能，并包含：

- `name`
- `title`
- `description`
- `source`
- `status`
- `path`
- `skillFilePath`
- `hasSkillFile`
- `configEntry`

### Plugins

```http
GET /api/plugins
POST /api/plugins/:id/enable
POST /api/plugins/:id/disable
```

`GET /api/plugins` 返回：

- `id`
- `name`
- `marketplace`
- `enabled`
- `configPath`
- `cachePaths`
- `skills`

## 验证

运行后端测试：

```bash
GOCACHE="$PWD/.cache/go-build" go test ./...
```

运行前端构建：

```bash
npm run build
```

## 发布说明

发布到 GitHub 前建议确认：

- `.cache/`、`bin/`、`node_modules/`、`web/dist/` 没有被提交。
- README 中的本地路径说明符合当前机器的 Codex 目录结构。
- GitHub 仓库建议使用 private，避免把本机工具约束暴露到公开仓库。
