# 🛡️ Teamie Avalon (`slug: "avalon"`) · 阿瓦隆圆桌裁判台

Teamie Marketplace 人机平权双通道群聊社交推理应用：**支持人类与 AI Agent 同桌博弈的阿瓦隆（The Resistance: Avalon）圆桌裁判台**。

- **零 IM 凭证**：公屏组队陈词、票型盘逻辑、真假梅林博弈与红方终局拍刀讨论全部走 Teamie 原生群聊消息流；本应用不持有也不调用任何 Teamie IM API。
- **对称身份与房间强隔离**：人类右侧抽屉 Web UI（`webUrl`）与群内在场 Eino Agent MCP Streamable HTTP（`mcpUrl`，`2025-03-26`）共享同一套 Go 状态机与 `HS256` Context Token 强制验签，按 `(workspaceId, conversationId)` 隔离房间，一切身份严格取自已验签 JWT `sub`。
- **字节级非对称视野迷雾（Fog-of-War）**：
  - **梅林（`MERLIN`）**：可见除莫德雷德（`MORDRED`）外的所有红方座位（含奥伯伦），不暴露具体红方神职。
  - **派西维尔（`PERCIVAL`）**：可见 `[梅林, 莫甘娜]` 候选座位 `merlinCandidateSeats`，按 `(gameId, viewerSub)` 确定性混洗，零角色顺序泄露且防刷新探测。
  - **亚瑟忠臣（`SERVANT`）**：无额外视野。
  - **红方常规爪牙（`ASSASSIN` / `MORGANA` / `MORDRED`）**：彼此互认，不可见奥伯伦。
  - **奥伯伦（`OBERON`）**：红方孤狼，与红方同伴双向不可见。
- **公开组队公投 vs 绝对匿名任务暗票**：
  - `TEAM_VOTE` 暗投集齐后公开全员赞成/反对票型并永久载入历轮档案；同一轮连续 `5` 次流票红方直接获胜（`EVIL_WIN_FIVE_REJECTS`）。
  - `QUEST_EXECUTION` 服务端强制拦截好人投失败票，且内部仅记录聚合票数 `(successCount, failCount)`，任何接口与内存结构均不保存座位出票映射。
  - 完整支持 `5–10` 人矩阵（`7–10` 人局第 4 轮自动开启需 `>= 2` 张失败票的保护轮）、单人顺时针轮流麦序（`PROPOSAL_SPEECH`，队长末置总结 + `publicChatPolicy` 拟人短句引导）以及终局刺客拍刀梅林（`ASSASSINATION`）。

---

## 目录结构

- `cmd/avalon/main.go`：统一 Go 服务端入口（内嵌 Vite 构建产物、REST/Web SSE 与 MCP Streamable HTTP）
- `internal/auth/`：对称 `HS256` Context Token 验签器（强校验 `alg=HS256`、`iss="teamie"`、`aud=TEAMIE_APP_ID`、`exp`）
- `internal/game/`：并发安全阿瓦隆核心状态机（支持 `5–10p` 预设与角色开关微调、非对称迷雾裁剪、阶段超时自动推进、`Idempotency-Key` 幂等重放保护）
- `internal/mcpserver/`：MCP Streamable HTTP (`2025-03-26`) 服务端，含 10 个正交工具（`get_my_view`、`join_lobby`、`leave_lobby`、`start_game`、`propose_team`、`finish_speech`、`vote_team`、`submit_quest_card`、`assassinate`、`reset_lobby`）、失效 `Mcp-Session-Id` 返回 `404` 重握手恢复与 `< 2 KB` 迷雾定向通知
- `internal/httpapi/`：Web UI 窄栏 REST + Bearer SSE 流接口与本地 `/dev` 多视角联调台
- `web/`：React + TypeScript (Vite) 窄屏优先（`320px–380px`，深浅色自适应）抽屉前端与 `postMessage` 桥接器
- `deploy/` & `ops/` & `.github/workflows/`：与 `zzqDeco/teamie` 一致的单机多端口容器化生产编排（默认 `:9444` HTTPS）、`release/v*` 不可变发布与一键回滚流水线

---

## 本地开发与多视角联调台（`/dev`）

```bash
# 1. 构建前端产物
cd web && npm ci && npm run lint && npm test && npm run build && cd ..

# 2. 运行全量竞态、防作弊与四阶段拨测红线测试
go test -race -v ./...

# 3. 启动本地开发与 5–7 人多视角联调台（仅 DEV_SIMULATOR=true 时开启 /dev）
DEV_SIMULATOR=true PORT=8080 go run ./cmd/avalon
# 浏览器打开 http://127.0.0.1:8080/dev 即可同屏并排测试 5–7 个真实 iframe 视角
```

---

## 生产自动化发布（与 `zzqDeco/teamie` 对齐）

向远端推送语义化发布分支 `release/vX.Y.Z`（源提交必须指向当前 `main`）即可触发 GitHub Actions 全自动校验、构建、SSH 部署至 `/srv/teamie-avalon`（独立监听公网 HTTPS `:9444`，与主站 `:443` 跨源隔离）、健康拨测与 Release 归档：

```bash
# 安全注入生产服务上的 Teamie 应用密钥（经 stdin 传输，切勿提交进 Git）
TEAMIE_APP_ID="<app.id>" TEAMIE_APP_SIGNING_SECRET="tmsec_..." bash ops/update-credentials.sh root@<ECS_IP>
```
