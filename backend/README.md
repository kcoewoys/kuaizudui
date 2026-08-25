# kuaizudui backend

Go + Gin + Gorm + MySQL + Redis 后端。启动时自动迁移数据库结构，福袋队列使用 Redis List 和 Lua 脚本保证 FIFO、跳过本人内容并原子防重。

## 配置

实际运行配置位于 [`config/config.yaml`](config/config.yaml)，生产参考位于 [`config/config.example.yaml`](config/config.example.yaml)。配置包含：

Docker Compose 会把本机的 `config/config.yaml` 只读挂载到 API 容器，并使用 `qrcode_uploads` 数据卷持久化上传的二维码。修改 `business.admin_phone` 或二维码上传限制后重启 `api` 服务即可生效。

- HTTP 地址、端口、模式、超时和允许的前端域名
- MySQL DSN 与连接池
- Redis 地址、密码、库号和超时
- 管理员手机号、二维码上传目录与大小限制、活动内容长度和福袋码长度
- 首次访问、福袋占用和管理员会话有效期
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
| `GET` | `/api/v1/activity/events` | SSE 实时推送本人活动轮次更新 |
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
普通额度 = 领码次数 - 普通轮数        ordinary_credit   = used_count - ordinary_rounds
插队额度 = 已投入积分 - 插队轮数      priority_credit   = boost_points_used - boost_rounds
```

额度归零时页面上的两个数字必然相等：普通队列显示“X 轮 / Y 次领码”（X = 普通轮数，Y = 领码次数），额度为 0 时 X == Y；插队队列显示“X 轮 / Y 积分”，同理。

### 状态转移

- **首次发布**：`used_count = 0`、`ordinary_credit = 0`——发布本身不产生领码次数；照常进入普通队列参与轮转。重新发布（修改内容）只更新内容，不改变任何计数。
- **点击一键领码且领到内容**：领取人 `used_count + 1`、`ordinary_credit + 1`，计数与领取在同一事务内提交。暂无可领内容时领取失败并返回“暂时没有可领取的内容”，不产生任何计数。
- **内容被领取**：命中普通队列时发布者 `ordinary_rounds + 1`、`ordinary_credit - 1`；命中插队队列时 `boost_rounds + 1`、`priority_credit - 1`。
- **额度归零**：成员保留在队列原位但被跳过（停泊）；再次领码后从原位置恢复参与。
- **不能领取自己发布的内容**（选取时跳过本人）。
- **同一发布者的内容每人只能领取一次**：`activity_claims` 以（领取人、活动类型、发布者）唯一约束记录每次成功领取；记录在领取事务内写入，并发重复领取撞唯一键后整体回滚并跳过该候选人继续查找，不会错误扣减对方额度。

### Redis 队列结构

每个队列由一个 Sorted Set 和一个计数 Hash 组成：

```text
activity_queue:{activityType}:{queueType}:zset     member = uid，score = ±序号
activity_queue:{activityType}:{queueType}:counts   hash，uid → 队列计数
activity_queue:seq                                  全局自增序号（首次使用时按毫秒时间戳播种防回拨）
activity_queue:{activityType}:seeded                队列已按 MySQL 初始化的标记
```

`queueType` 为 `ordinary` 或 `priority`。score 编码承载两个事实：

- **绝对值 = 入队序号**，决定 FIFO 位置，一经分配永不改变；额度归零再恢复时仍占原位。
- **符号 = 参与开关**：正数表示计数 > 0、可被选取；负数表示计数已归零、原地停泊跳过。

计数加减由 Lua 原子完成，跨越 0 边界时只翻转符号、不触碰位置；选取使用 `ZRANGEBYSCORE (0 +inf LIMIT 0 N` 取最早入队的活跃成员并跳过本人与已领过的发布者，全部操作 O(log N)，停泊的死成员不会阻塞队头。

### 候选选择顺序

1. 校验领取人已发布当前活动类型的内容。
2. 确保 Redis 队列已从 MySQL 初始化（未初始化则清空重建）。
3. 从插队队列选取最早入队的、非本人且未领过的活跃成员。
4. 插队队列无候选人（为空、只剩本人或都已领过）时落到普通队列。
5. 在 MySQL 事务中对候选人行加锁、复验额度、写入领取记录并扣减额度，返回其内容。
6. 候选记录或额度失效时将其移出队列后重试；命中“已领过”时加入排除列表重试（不从队列移除）。

### Redis 重建与一致性

API 启动时清空所有活动类型的 Redis 队列和 `seeded` 标记；每个活动第一次领取时按 MySQL 全量重建队列（含每个成员的当前额度）。运行期额度增减的 Redis 操作失败会使 `seeded` 失效，下一次请求触发清空重建自愈。Redis 丢失或重启不会丢失轮数与额度数据。

### 领取按钮状态

`can_claim` 只检查 MySQL 中是否存在其他用户发布的当前活动内容，不看额度：

```text
type = 当前活动 AND uid != 当前用户
```

只有自己发布时按钮置灰；他人额度耗尽或都已被本人领取时按钮仍可点击，点击后提示暂无可领内容，领取失败、不累计领码次数。

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

二维码仅接受真实 PNG/JPEG 图片，默认最大 5 MB、最大边长 6000 px。后端通过文件签名和图片头校验内容，不信任扩展名或浏览器传入的 MIME 类型。

## 验证

```bash
go test ./...
go vet ./...
```

测试使用 SQLite 和 `miniredis` 作为本地可替代适配器，覆盖实际数据库事务、Redis Lua、HTTP 路由和管理员鉴权。
