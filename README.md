# Codex Skill Manager

Codex Skill Manager 是一个本地 Web 管理工具，用来查看和管理本机 Codex 的 skills 与 plugins。它适合已经安装了多个 Codex skills/plugins，希望用图形界面快速了解来源、状态、路径，并安全地启用、禁用或清理用户自装技能的用户。

项目默认只在本机运行，不需要数据库，不会上传你的 Codex 配置或技能内容。

## 你可以用它做什么

- 查看当前 Codex skills，并按插件、用户自装、系统技能分组。
- 查看每个 skill 的名称、描述、来源、状态、路径和对应的 `SKILL.md`。
- 启用或禁用 skill，操作会写入 Codex 原生配置 `config.toml`。
- 查看 Codex plugins，以及每个 plugin 带来的 skills。
- 启用或禁用 plugin，操作只修改目标 plugin 配置块。
- 软删除用户自装 skill，目录会移动到 trash，而不是直接永久删除。
- 迁移早期通过目录移动方式禁用的 legacy skills。

## 界面预览

应用启动后直接进入管理界面，没有登录页或营销页。技能页按层级展示：

- 插件分组，例如 `github@openai-curated`
- 用户自装技能
- 系统技能

展开分组后可以查看该组下的具体 skills。插件页展示已配置插件、缓存路径数量和插件提供的 skills。

## 安装与运行

前置要求：

- Go 1.24 或更新版本
- Node.js 20 或更新版本
- npm
- 本机已有 Codex 配置目录，默认位于 `~/.codex`

克隆项目：

```bash
git clone https://github.com/Driftlux/skill-manager.git
cd skill-manager
```

安装前端依赖：

```bash
npm install
```

构建前端：

```bash
npm run build
```

启动本地服务：

```bash
go run ./cmd/skill-manager -addr 127.0.0.1:5174 -static-dir web/dist
```

打开浏览器访问：

```text
http://127.0.0.1:5174
```

## 常用操作

### 启用或禁用技能

在技能页展开分组，找到目标 skill，点击“启用”或“禁用”。

这个操作不会移动技能目录，也不会修改 `SKILL.md` 内容。应用会更新：

```toml
[[skills.config]]
path = "/path/to/SKILL.md"
enabled = false
```

如果禁用项不存在，禁用时会追加到 `config.toml` 末尾。启用时会把对应项改为 `enabled = true`。

### 启用或禁用插件

在插件页找到目标 plugin，点击“启用”或“禁用”。

插件禁用前会弹出确认提示，因为禁用插件可能会隐藏该插件带来的 skills、tools、connectors、apps 和 hooks。

应用只修改目标块：

```toml
[plugins."vercel@openai-curated"]
enabled = false
```

不会删除插件缓存。

### 软删除用户自装技能

只有 `source = user` 的技能允许删除。删除前会显示完整路径并要求二次确认。

删除不是永久删除，目录会移动到：

```text
~/.codex/skills.trash/<timestamp>-<skill-name>
```

系统技能和插件技能不会显示删除按钮。

## 安全设计

这个工具管理的是本机真实 Codex 配置，因此默认采用保守策略：

- 每次写入 `config.toml` 前都会创建备份：`config.toml.bak.<timestamp>`。
- 技能启用/禁用只修改 `[[skills.config]]`。
- 插件启用/禁用只修改目标 `[plugins."<id>"]` 块。
- 不会重排或重写整个 `config.toml`。
- 不会修改 `SKILL.md` 内容。
- 不会删除系统技能目录。
- 不会删除插件缓存目录。
- 不会读取或修改 Codex memories。
- 所有路径都会做安全校验，避免路径穿越。

默认路径：

```text
~/.codex/config.toml
~/.codex/skills/
~/.codex/skills/.system/
~/.codex/plugins/cache/
~/.codex/skills.trash/
```

禁止修改或删除：

```text
~/.codex/skills/.system/
~/.codex/plugins/cache/
~/.codex/memories/
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

`GET /api/skills` 返回：

- `name`
- `title`
- `description`
- `source`: `user | system | plugin`
- `status`: `enabled | disabled | invalid`
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

## 开发

运行后端测试：

```bash
GOCACHE="$PWD/.cache/go-build" go test ./...
```

构建前端：

```bash
npm run build
```

开发时可以分别运行 Vite 和 Go 服务，也可以先构建前端后由 Go 服务托管 `web/dist`。

## 适用范围

当前版本专注于本机 Codex skills/plugins 管理。它不是通用插件市场，也不会安装新插件或新技能。

如果你只是想安全地看清楚“当前有哪些 skills/plugins、它们来自哪里、是否启用”，这个工具就是为这个场景设计的。
