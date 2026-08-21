# CW Remind Plugin 需求与设计规格

## 1. 目标

CW Remind 是 Mattermost 服务器插件。用户在频道中输入 `/remind` 后打开设置窗口，建立一条包含多个提醒时点的期限提醒。

## 2. MVP 需求

### 2.1 建立提醒

- 触发：在任意有权限的频道输入 `/remind`。
- 字段：
  - 提醒内容：必填，1–4000 字符。
  - 标题：必填，1–200 字符，作为 DueWatch 帖子的主标题。
  - 执行期限：必填，日期（`YYYY-MM-DD`）。
  - 提醒对象：`all`、`channel` 或指定 Mattermost 用户。
  - 指定用户：仅选择“个别用户”时显示，可搜索并多选当前频道成员。
  - 提醒规则：至少一组，每组为“期限前 N 天 + HH:mm”；N 可为 0（当天）。
- 动态操作：可追加或删除规则。同一提醒不允许重复的 `N + HH:mm`。
- 时区：以建立者浏览器的 IANA 时区计算，并随数据保存。
- GUI 语言：跟随当前用户的 Mattermost locale，支持英语（默认）、日语和简体中文；其他 locale 回退到英语。
- 历史记录：在同一窗口中显示当前频道的全部提醒事件，按建立时间倒序排列，并显示创建者、规则发送状态及目标用户处理状态。
- 管理：仅创建者可以编辑或删除自己的提醒；其他频道成员只有查看权限。

### 2.2 投递语义

- 插件激活时创建或复用专用 `DueWatch` Bot（用户名 `duewatch`），设置喇叭头像，所有提醒均由该 Bot 发布。
- `all`：在建立时的频道发布并带 `@all`。
- `channel`：在建立时的频道发布并带 `@channel`。
- `users`：在建立时的频道发布并 @ 所有选定用户。
- 建立成功后立即发布公告，直接显示用户标题；正文使用安全的 Markdown 代码围栏，另含期限、建立者和所有后续提醒日期时间。
- 建立时保存目标用户快照：`users` 为所选成员，`all/channel` 为当时频道内全部有效非 Bot 成员。
- 公告及后续提醒提供“了解”和“対応済み”按钮；“了解”表示已知晓但尚未处理，用户之后可以升级为“対応済み”。
- 后续提醒只 @ 尚未“対応済み”的目标用户，并显示“未対応 / 了解 / 対応済み”名单；全部处理完成后不再发送后续帖子。
- 编辑事件会重新计算未来规则，并同步更新该事件已发布的所有帖子内容；仍属于目标成员的处理状态会保留。
- 删除采用软删除：停止调度、保留履历，并给该事件已发布的帖子内容增加删除线。
- 调度器每 30 秒扫描一次；已成功投递的规则标记为已发送，避免正常轮询中重复。

### 2.3 校验与权限

- GUI API 请求必须带 Mattermost 注入的 `Mattermost-User-ID`，未登录请求返回 401。
- 交互按钮回调校验 Mattermost 提供的用户、频道和帖子，并使用不对客户端 API 暴露的私密令牌防止伪造。
- 只能在用户当前所属频道建立提醒。
- 指定用户必须是当前频道的有效 Mattermost 成员。
- 不接受已过期的全部规则；如部分时点已过，返回字段错误，由用户修正。

## 3. 交互流程

1. 服务端注册 `/remind` Slash Command。
2. 命令执行时，服务端通过定向 WebSocket 事件发送 `channel_id`。
3. WebApp 收到事件后打开 React 模态窗口。
4. 用户提交后，WebApp 调用 `POST /plugins/com.cw.remind/api/v1/reminders`。
5. 服务端校验、计算 UTC 触发时间，由 `DueWatch` 发布建立公告，写入 KV Store，并返回 201。
6. 目标用户点击“了解”或“対応済み”后，服务端记录用户、状态与时间并更新帖子状态。
7. 调度器到期后由 `DueWatch` Bot 在原频道提醒尚未处理的成员；无人未处理时仅将规则标记完成。

## 4. 数据模型

```text
Reminder
  id, creator_id, creator_username, channel_id
  content, due_date, timezone
  title
  audience: all | channel | users
  usernames[]
  target_users[]: {id, username}
  acknowledgements[]: {user_id, username, acknowledged_at}
  completions[]: {user_id, username, completed_at}
  rules[]: {days_before, time, fire_at, fired_at?}
  created_at, updated_at, deleted_at, deleted_by
  posts[]: {id, announcement}, announcement_post_id, action_token
```

MVP 使用 Plugin KV Store：每条数据一个 key，另有一个 ID 索引。写入由进程锁保护。这适合单节点/小规模 MVP；生产 HA 扩展见第 6 节。

## 5. API

### `POST /api/v1/reminders`

```json
{
  "channel_id": "channel-id",
  "title": "月度报告",
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

响应中的 Reminder 不包含内部 `action_token`。

### `GET /api/v1/reminders?channel_id={channel-id}`

返回指定频道的全部提醒事件，按建立时间倒序。

### `GET /api/v1/channel-users?channel_id={channel-id}&q={query}`

返回当前频道的有效非 Bot 成员；`q` 可选，用于按用户名或显示名搜索，供 GUI 多选器使用。

### `POST /api/v1/actions/complete`

Mattermost 交互消息内部回调。记录点击用户的处理状态并更新原帖子，不供 WebApp 直接调用。

### `POST /api/v1/actions/acknowledge`

Mattermost 交互消息内部回调。将目标用户标记为“了解”，但用户仍属于后续提醒对象。

### `PUT /api/v1/reminders/{id}` / `DELETE /api/v1/reminders/{id}`

仅事件创建者可调用。编辑更新事件及已有帖子；删除软删除事件并给已有帖子加删除线。

## 6. 后续扩展设计

- 管理：暂停，以及按发送状态筛选历史。
- HA：将调度改为 cluster mutex/job，用 CAS 领取任务，确保多节点只投递一次。
- 可靠性：投递失败指数退避，死信状态，管理员可观测指标。MVP 在“发帖成功、但写入 fired_at 前进程崩溃”的极端情况可能重发；严格 exactly-once 需要幂等投递标识或事务性 outbox。
- 频道变更：频道删除/归档时标记任务失效并通知建立者。
- 重复周期：支持每周、每月、Cron 表达式及工作日日历。
- 权限：管理员可禁用 `@all`，设置单用户配额和最大提前天数。

## 7. 验收标准

- `/remind` 只给命令发起者打开设置窗口。
- 可建立“提前 7 天、提前 3 天、当天”三组不同时间的规则。
- 服务器重启后未发送任务仍存在。
- 正常运行下，每条规则只成功发送一次。
- 非法日期、时间、时区、用户或过期规则不会被保存。
- 建立后频道立即出现正文、期限和后续提醒时间。
- 非目标用户不能确认；目标用户确认后，帖子和历史记录显示处理状态。
- 后续提醒仅点名未处理用户，全部处理后不再发送后续帖子。
