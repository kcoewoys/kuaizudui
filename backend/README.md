# kuaizudui backend

Go + Gin + Gorm + MySQL + Redis 后端。启动时自动创建缺失的数据库并迁移表结构，福袋队列使用 Redis List 和 Lua 脚本保证 FIFO、跳过本人内容并原子防重。

## 配置

实际运行配置位于 [`config/config.yaml`](config/config.yaml)，生产参考位于 [`config/config.example.yaml`](config/config.example.yaml)。配置包含：

Docker Compose 会把本机的 `config/config.yaml` 只读挂载到 API 容器，并使用 `qrcode_uploads` 数据卷持久化上传的二维码。修改 `business.admin_phone` 或二维码上传限制后重启 `api` 服务即可生效。

- HTTP 地址、端口、模式、超时和允许的前端域名
- MySQL DSN 与连接池
- Redis 地址、密码、库号和超时
- 管理员手机号、二维码上传目录与大小限制、活动内容长度和福袋码长度
- 首次访问、福袋占用、管理员会话有效期和每日重置时间
- 管理员令牌签名密钥

部署时可以用以下环境变量覆盖敏感或环境相关字段：

| 环境变量 | 配置项 |
| --- | --- |
| `APP_SERVER_HOST` | `server.host` |
| `APP_SERVER_PORT` | `server.port` |
| `APP_SERVER_MODE` | `server.mode` |
| `APP_MYSQL_DSN` | `mysql.dsn` |
| `APP_REDIS_ADDRESS` | `redis.address` |
| `APP_REDIS_PASSWORD` | `redis.password` |
| `APP_REDIS_DATABASE` | `redis.database` |
| `APP_ADMIN_PHONE` | `business.admin_phone` |
| `APP_QRCODE_UPLOAD_DIR` | `business.qrcode_upload_dir` |
| `APP_QRCODE_MAX_UPLOAD_BYTES` | `business.qrcode_max_upload_bytes` |
| `APP_ADMIN_TOKEN_SECRET` | `security.admin_token_secret` |

`release` 模式禁止使用仓库中的默认签名密钥。

## 本地运行

只启动依赖：

```bash
docker compose up -d mysql redis
go run ./cmd/server -config config/config.yaml
```

启动完整后端：

```bash
docker compose up --build
```

健康检查：

```bash
curl http://127.0.0.1:8080/health/live
curl http://127.0.0.1:8080/health/ready
```

## 用户身份

用户接口使用 `X-UID` 请求头。第一次请求可以不传，后端会生成 UID，并在响应的 `X-UID` 中返回；前端应保存并在后续请求中带回。

```bash
curl -i http://127.0.0.1:8080/api/v1/user/info
curl -H 'X-UID: 上一步返回的值' http://127.0.0.1:8080/api/v1/points
```

## 存储设计

MySQL 是最终事实来源：账号、积分、轮数与额度、福袋码状态都落库。Redis 只保存可重建或可过期的调度态（队列顺序、占用锁、会话、标记），任何 Redis 数据丢失都可以从 MySQL 自愈或按 TTL 自然过期。

### MySQL 表

| 表 | 用途 | 每日重置 |
| --- | --- | --- |
| `users` | 匿名账号：UID（唯一）、绑定的手机号（唯一）、邀请人 UID、积分余额 | 保留 |
| `activity_contents` | 每用户每活动一条：发布内容与全部轮数/额度计数（字段含义见「活动领取双队列」） | 清空 |
| `activity_claims` | 领取记录，(领取人, 活动类型, 发布者) 唯一，保证同一发布者的内容每人只领一次 | 清空 |
| `lucky_codes` | 福袋码：码值唯一、状态（available/used）、使用者与使用时间 | 清空 |
| `point_records` | 积分流水：每笔加/扣积分的来源与说明 | 保留 |
| `recharge_records` | 管理员充值流水 | 保留 |
| `exchange_codes` | 积分兑换码与使用状态 | 保留 |
| `notices` | 公告，按 `type` 主键 | 保留 |
| `settings` | 系统设置：数据库迁移标记、群二维码文件名（`group_qrcode`）、每日重置标记（`daily_reset.last_run`） | 保留 |
| `feedback` | 用户反馈 | 保留 |

### Redis 键

| 键 | 类型 | 用途 |
| --- | --- | --- |
| `activity_queue:{type}:ordinary:zset`、`activity_queue:{type}:priority:zset` | ZSet | 队列成员（member = uid）；score 绝对值 = 入队序号定 FIFO 位，符号 = 额度是否大于 0 |
| `activity_queue:{type}:ordinary:counts`、`activity_queue:{type}:priority:counts` | Hash | uid → 队列剩余额度 |
| `activity_queue:{type}:cursor` | String | 普通队列共享 FIFO 轮询游标；只被成功领取推进，服务到队尾后归零回到队首 |
| `activity_queue:seq` | String | 全局入队序号发号器（首次使用时按毫秒时间戳播种防回拨） |
| `activity_queue:{type}:seeded` | String | 队列已按 MySQL 播种的标记 |
| `lucky_queue` | List | 待领取福袋码（`id\|uid`），LPUSH + RPOP 实现 FIFO，Lua 原子跳过本人并防重 |
| `lucky_used:{id}` | String | 福袋码领取占用，TTL = `lucky_claim_ttl`（默认 24h） |
| `admin_session:{token}` | String | 管理员会话（值 = 手机号），TTL = `admin_session_ttl`（默认 12h） |
| `first_visit:{uid}` | String | 首次访问标记，TTL = `first_visit_ttl`（默认 365 天） |
| `activity_updates:{uid}` | Pub/Sub | SSE 实时通知频道（本人内容被领取），不落盘、重置无影响 |
| `activity_updates:activity:{type}` | Pub/Sub | SSE 活动全员广播频道：他人发布、插队、领取成功后触发，刷新各端可领状态 |

### 每日重置

`business.daily_reset_time`（默认 `00:00`，服务器本地时区）每天触发一次，动作有两步：

1. `FLUSHDB` 清空整个 Redis 库——活动队列、福袋队列与占用、管理员会话、首访标记全部失效。
2. 事务删除 `activity_contents`、`activity_claims`、`lucky_codes` 三张当日业务表的全部行。

执行标记写在 MySQL `settings` 表（`daily_reset.last_run` = 日期）：服务重启不会把当天已重置过的再清一遍；重置时刻服务恰好不在运行，下次启动会补跑；执行失败每分钟重试，不会跳过当天。后果需要知晓：管理员每天需重新登录；首访标记每日清空，每个用户每天都会再次判定为首次访问；未用完的插队额度随当日数据作废、不退积分；已使用福袋码的历史不留痕。

## 接口

统一响应格式：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

### 用户端

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/user/info` | 用户、手机号、积分和首次访问状态 |
| `POST` | `/api/v1/user/bind-phone` | 绑定手机号 |
| `POST` | `/api/v1/user/referral` | 按邀请链接中的手机号记录邀请人；邀请关系只能建立一次 |
| `POST` | `/api/v1/lucky/publish` | 发布 8–9 位福袋码 |
| `GET` | `/api/v1/lucky/list` | 获取可用福袋码列表；码与发布者 ID 脱敏，并标记本人发布 |
| `POST` | `/api/v1/lucky/receive` | FIFO 一键领取并返回完整福袋码 |
| `POST` | `/api/v1/lucky/use` | 领取指定福袋码 |
| `POST` | `/api/v1/activity/publish` | 发布买菜或现金活动内容 |
| `GET` | `/api/v1/activity/detail?type=...` | 查询自己的活动内容 |
| `POST` | `/api/v1/activity/boost` | 提交积分并加入活动插队队列 |
| `POST` | `/api/v1/activity/use` | 优先从插队队列、再从普通队列领取其他用户内容；不能领取本人的及已领过的 |
| `GET` | `/api/v1/activity/events?type=...` | SSE 实时推送活动更新：本人内容被领取，带 `type` 时同时订阅该活动的全员广播（他人发布/插队/领取） |
| `GET` | `/api/v1/points` | 查询积分 |
| `GET` | `/api/v1/points/history` | 查询加分来源记录；不返回扣分和零积分记录 |
| `POST` | `/api/v1/exchange` | 使用兑换码 |
| `GET` | `/api/v1/notices/:type` | 查询公告 |
| `GET` | `/api/v1/group-qrcode` | 查询交流群二维码 |
| `GET` | `/api/v1/group-qrcode/image` | 读取当前上传的二维码图片 |

活动 `type` 支持：`buy_food`、`cash_turntable`、`cash_monopoly`、`daily_cash`。四种类型走同一业务模块和同一套 200 字校验。

## 活动领取双队列

四种活动共用同一套“普通队列 + 插队队列”领取模型：发布进入普通队列，使用积分进入插队队列；领取时先消费插队队列，插队队列没有可选候选人时落到普通队列。MySQL 保存轮数和额度并作为最终事实来源，Redis 保存调度顺序与队列计数。

### 计数规则

每个用户在每种活动下只有一条 `activity_contents` 记录：

| 领域含义 | 数据库字段 | 说明 |
| --- | --- | --- |
| 领码次数 | `used_count` | 每次成功领取到内容 +1；无可领内容即领取失败、不计数 |
| 普通轮数 | `ordinary_rounds` | 本人内容被其他用户从普通队列领取的累计次数 |
| 普通额度 | `ordinary_credit` | 本人内容还能被领取的次数，即普通队列中的计数 |
| 插队轮数 | `boost_rounds` | 本人内容被其他用户从插队队列领取的累计次数 |
| 已投入积分 | `boost_points_used` | 本人累计用于插队的积分 |
| 插队额度 | `priority_credit` | 本人内容还能从插队队列被领取的次数 |

两个队列遵守同一形式的不变量（额度即队列计数）：

```text
普通额度 = 发布赠送次数 + 领码次数 - 普通轮数    ordinary_credit   = activity_publish_ordinary_credit + used_count - ordinary_rounds
插队额度 = 已投入积分 - 插队轮数                priority_credit   = boost_points_used - boost_rounds
```

其中「发布赠送次数」由 `business.activity_publish_ordinary_credit` 配置，默认 3，设为 0 表示发布不赠送。

额度归零时页面上的两个数字必然相等：普通队列显示“X / Y 次”（X = 已被领取的次数，Y = X + 剩余次数，即发布赠送次数加上自己领码攒出的机会），额度为 0 时 X == Y；插队队列显示“X 轮 / Y 积分”，同理。

### 状态转移

- **首次发布**：`used_count = 0`、`ordinary_credit` 按发布赠送次数配置赠送——发布即以活跃状态进入普通队列轮转。重新发布（修改内容）只更新内容，不改变任何计数，次数用尽后不能靠编辑内容恢复。
- **点击一键领码且领到内容**：领取人 `used_count + 1`、`ordinary_credit + 1`，计数与领取在同一事务内提交。没有可领的新内容时（所有发布者都已服务过本人、他人次数用尽或队列只有本人）领取失败并返回“暂时无码可领”，不产生任何计数。
- **内容被领取**：命中普通队列时发布者 `ordinary_rounds + 1`、`ordinary_credit - 1`；命中插队队列时 `boost_rounds + 1`、`priority_credit - 1`。
- **额度归零**：成员保留在队列原位但被跳过（停泊）；再次领码后从原位置恢复参与。
- **不能领取自己发布的内容**（选取时跳过本人）。
- **同一发布者的内容每人只能领取一次**：`activity_claims` 以（领取人、活动类型、发布者）唯一约束记录每次成功领取；记录在领取事务内写入，并发重复领取撞唯一键后整体回滚并跳过该候选人继续查找，不会错误扣减对方额度。

### Redis 队列结构

每个队列由一个 Sorted Set 和一个计数 Hash 组成：

```text
activity_queue:{activityType}:{queueType}:zset     member = uid，score = ±序号
activity_queue:{activityType}:{queueType}:counts   hash，uid → 队列计数
activity_queue:{activityType}:cursor               普通队列的共享 FIFO 游标
activity_queue:seq                                  全局自增序号（首次使用时按毫秒时间戳播种防回拨）
activity_queue:{activityType}:seeded                队列已按 MySQL 初始化的标记
```

`queueType` 为 `ordinary` 或 `priority`。score 编码承载两个事实：

- **绝对值 = 入队序号**，决定 FIFO 位置，一经分配永不改变；额度归零再恢复时仍占原位。
- **符号 = 参与开关**：正数表示计数 > 0、可被选取；负数表示计数已归零、原地停泊跳过。

计数加减由 Lua 原子完成，跨越 0 边界时只翻转符号、不触碰位置。插队队列选取使用 `ZRANGEBYSCORE (0 +inf LIMIT 0 N` 取最早入队的活跃成员并跳过本人与已领过的发布者；普通队列由共享游标 `cursor` 按发布顺序轮转，**只被成功领取推进**——跳过本人、已领过的发布者与已停泊（次数用尽）的成员，走到队尾后归零回到队首继续找未领过的活跃成员；当所有发布者都已服务过该领取人时返回空，领取以“暂时无码可领”结束，绝不重复发放。发布新成员只追加到队尾：游标走完一圈后指向队首，新发布者排在轮转末尾，绝不会插到未被服务过的成员前面。成员不会被普通领取移除，停泊者攒回次数后从原位置恢复参与。全部操作 O(log N)，停泊的死成员不会阻塞队头。

### 候选选择顺序

1. 校验领取人已发布当前活动类型的内容。
2. 确保 Redis 队列已从 MySQL 初始化（未初始化则清空重建）。
3. 从插队队列选取最早入队的、非本人且未领过的活跃成员。
4. 插队队列无候选人（为空、只剩本人或都已领过）时落到普通队列。
5. 在 MySQL 事务中对候选人行加锁、复验额度、写入领取记录并扣减额度，返回其内容。
6. 候选记录或额度失效时将其移出队列后重试；命中“已领过”时加入排除列表重试（不从队列移除）。

### Redis 重建与一致性

API 启动时清空所有活动类型的 Redis 队列和 `seeded` 标记；每个活动第一次领取时按 MySQL 全量重建队列（含每个成员的当前额度）。运行期额度增减的 Redis 操作失败会使 `seeded` 失效，下一次请求触发清空重建自愈。Redis 丢失或重启不会丢失轮数与额度数据。每日重置（见「存储设计」）会整库清空 Redis 并删除当日业务表，队列随后从空表重新播种，等价于一次彻底的冷启动。

### 领取按钮状态

`can_claim` 检查 MySQL 中是否存在“本人未领取过、且还持有普通或插队额度”的其他发布者：

```text
type = 当前活动 AND uid != 当前用户
AND (ordinary_credit > 0 OR priority_credit > 0)
AND uid NOT IN (该用户在本活动的领取记录中的发布者)
```

只有自己发布、他人额度全部耗尽、或可领者都已被本人领取过时按钮置灰。实时性由广播事件保证：他人发布、插队、领取成功（额度变动）都会向 `activity_updates:activity:{type}` 广播，各端收到后刷新详情，按钮状态即时变化；事件丢失时以下次详情查询为准。

### 主要实现位置

- `internal/platform/platform.go`：领码计数、候选选择、事务与领取记录、可领取状态判断。
- `internal/queue/activity.go`：ZSet / counts / seq 键、符号位翻转、带排除列表的选取 Lua 脚本。
- `internal/domain/models.go`：活动额度字段与 `activity_claims` 领取记录模型。
- `internal/platform/platform_test.go`、`internal/queue/activity_test.go`：冷启动互领、停泊恢复、插队优先、不可重复领取同一发布者等回归测试。

### 管理端

管理端没有独立密码登录页。访问前端 `/admin` 时会弹出手机号验证框，`POST /api/v1/admin/login` 仅接受配置文件 `business.admin_phone`（或 `APP_ADMIN_PHONE`）中的号码。验证成功后，其余接口需携带 `Authorization: Bearer <token>`。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/admin/logout` | 注销会话 |
| `GET` | `/api/v1/admin/users` | 查询用户，支持 `q/limit/offset` |
| `POST` | `/api/v1/admin/recharge` | 给手机号充值积分 |
| `GET` | `/api/v1/admin/recharges` | 查看充值记录 |
| `POST` | `/api/v1/admin/notice` | 更新公告 |
| `POST` | `/api/v1/admin/exchange/create` | 批量生成兑换码，单次最多 500 个 |
| `GET` | `/api/v1/admin/exchanges` | 查看兑换码及使用状态 |
| `POST` | `/api/v1/admin/qrcode` | 上传交流群二维码，`multipart/form-data` 字段名为 `image` |
| `DELETE` | `/api/v1/admin/qrcode` | 移除当前交流群二维码 |
| `GET` | `/api/v1/admin/activity-queues` | 监视四种活动的普通/插队队列游标状态 |

二维码仅接受真实 PNG/JPEG 图片，默认最大 5 MB、最大边长 6000 px。后端通过文件签名和图片头校验内容，不信任扩展名或浏览器传入的 MIME 类型。

`GET /api/v1/admin/activity-queues` 返回四种活动的双队列快照，对应管理台「状态」栏目。每个队列包含四个字段：`created` 表示队列是否已创建（Redis 会自动删除空有序集合，键不存在即“队列未创建”）；`total` 为队列总个数（含额度归零的停泊成员）；`position` 为游标位置——普通队列是共享 FIFO 游标在队列中的 0 起算位次（游标尚未移动或走完一圈时为 0），插队队列按最早入队选取、没有游标，恒为 0；`cursor_seq` 是游标的原始入队序号（毫秒时间戳播种），同样仅普通队列非零。

## 验证

```bash
go test ./...
go vet ./...
```

测试使用 SQLite 和 `miniredis` 作为本地可替代适配器，覆盖实际数据库事务、Redis Lua、HTTP 路由和管理员鉴权。
