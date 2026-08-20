# CW Remind Plugin 需求与设计规格

## 1. 目标

CW Remind 是 Mattermost 服务器插件。用户在频道中输入 `/remind` 后打开设置窗口，建立一条包含多个提醒时点的期限提醒。

## 2. MVP 需求

### 2.1 建立提醒

- 触发：在任意有权限的频道输入 `/remind`。
- 字段：
  - 提醒内容：必填，1–4000 字符。
  - 执行期限：必填，日期（`YYYY-MM-DD`）。
  - 提醒对象：`all`、`channel` 或指定 Mattermost 用户。
  - 指定用户：仅选择“个别用户”时显示，输入一个或多个 `@username`，逗号分隔。
  - 提醒规则：至少一组，每组为“期限前 N 天 + HH:mm”；N 可为 0（当天）。
- 动态操作：可追加或删除规则。同一提醒不允许重复的 `N + HH:mm`。
- 时区：以建立者浏览器的 IANA 时区计算，并随数据保存。

### 2.2 投递语义

- `all`：在建立时的频道发布并带 `@all`。
- `channel`：在建立时的频道发布并带 `@channel`。
- `users`：在建立时的频道发布并 @ 所有选定用户。
- 消息包含提醒内容、期限、建立者。
- 调度器每 30 秒扫描一次；已成功投递的规则标记为已发送，避免正常轮询中重复。

### 2.3 校验与权限

- HTTP 请求必须带 Mattermost 注入的 `Mattermost-User-ID`，未登录请求返回 401。
- 只能在用户当前所属频道建立提醒。
- 指定用户必须是存在的 Mattermost 账号。
- 不接受已过期的全部规则；如部分时点已过，返回字段错误，由用户修正。

## 3. 交互流程

1. 服务端注册 `/remind` Slash Command。
2. 命令执行时，服务端通过定向 WebSocket 事件发送 `channel_id`。
3. WebApp 收到事件后打开 React 模态窗口。
4. 用户提交后，WebApp 调用 `POST /plugins/com.cw.remind/api/v1/reminders`。
5. 服务端校验、计算 UTC 触发时间，写入 KV Store，并返回 201。
6. 调度器到期后在原频道发布提醒。

## 4. 数据模型

```text
Reminder
  id, creator_id, creator_username, channel_id
  content, due_date, timezone
  audience: all | channel | users
  usernames[]
  rules[]: {days_before, time, fire_at, fired_at?}
  created_at
```

MVP 使用 Plugin KV Store：每条数据一个 key，另有一个 ID 索引。写入由进程锁保护。这适合单节点/小规模 MVP；生产 HA 扩展见第 6 节。

## 5. API

### `POST /api/v1/reminders`

```json
{
  "channel_id": "channel-id",
  "content": "提交报告",
  "due_date": "2026-09-01",
  "timezone": "Asia/Tokyo",
  "audience": "users",
  "usernames": ["alice", "bob"],
  "rules": [
    {"days_before": 7, "time": "09:00"},
    {"days_before": 3, "time": "15:30"},
    {"days_before": 0, "time": "09:00"}
  ]
}
```

成功返回 201 和完整 Reminder；校验失败返回 400 `{error, fields?}`。

## 6. 后续扩展设计

- 管理：`/remind list`、编辑、删除、暂停、已发送历史。
- 用户选择：改为调用 Mattermost 用户搜索 API 的自动完成多选器。
- HA：将调度改为 cluster mutex/job，用 CAS 领取任务，确保多节点只投递一次。
- 可靠性：投递失败指数退避，死信状态，管理员可观测指标。MVP 在“发帖成功、但写入 fired_at 前进程崩溃”的极端情况可能重发；严格 exactly-once 需要幂等投递标识或事务性 outbox。
- 频道变更：频道删除/归档时标记任务失效并通知建立者。
- 重复周期：支持每周、每月、Cron 表达式及工作日日历。
- 国际化：把中文 UI 文案改为 i18n 资源，增加日文/英文。
- 权限：管理员可禁用 `@all`，设置单用户配额和最大提前天数。

## 7. 验收标准

- `/remind` 只给命令发起者打开设置窗口。
- 可建立“提前 7 天、提前 3 天、当天”三组不同时间的规则。
- 服务器重启后未发送任务仍存在。
- 正常运行下，每条规则只成功发送一次。
- 非法日期、时间、时区、用户或过期规则不会被保存。
