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
| `POST` | `/api/v1/activity/use` | 优先从插队队列、再从普通队列领取其他用户内容 |
| `GET` | `/api/v1/activity/events` | SSE 实时推送本人活动轮次更新 |
| `GET` | `/api/v1/points` | 查询积分 |
| `GET` | `/api/v1/points/history` | 查询加分来源记录；不返回扣分和零积分记录 |
| `POST` | `/api/v1/exchange` | 使用兑换码 |
| `GET` | `/api/v1/notices/:type` | 查询公告 |
| `GET` | `/api/v1/group-qrcode` | 查询交流群二维码 |
| `GET` | `/api/v1/group-qrcode/image` | 读取当前上传的二维码图片 |

活动 `type` 支持：`buy_food`、`cash_turntable`、`cash_monopoly`、`daily_cash`。四种类型走同一业务模块和同一套 200 字校验。

## 活动领取双队列

活动领取使用“插队优先、普通兜底”的两级轮询模型。MySQL 是轮次和额度的最终事实来源，Redis 只保存当前可参与调度的用户 ID，不保存轮次数值。每个用户在每种活动类型下只有一条活动内容记录。

### 领域字段与不变量

`activity_contents` 中与队列有关的字段如下：

| 领域含义 | 数据库字段 | 说明 |
| --- | --- | --- |
| 领码次数 | `used_count` | 本人成功领取其他用户内容的累计次数 |
| 普通轮数 | `ordinary_rounds` | 本人内容从普通队列成功发给其他用户的累计次数 |
| 普通额度 | `ordinary_credit` | 本人内容还能从普通队列发出的次数 |
| 插队轮数 | `boost_rounds` | 本人内容从插队队列成功发给其他用户的累计次数 |
| 已投入积分 | `boost_points_used` | 本人累计确认用于插队的积分 |
| 插队额度 | `priority_credit` | 本人内容还能从插队队列发出的次数 |

普通队列始终遵守以下不变量：

```text
普通额度 = 领码次数 - 普通轮数
ordinary_credit = used_count - ordinary_rounds
```

因此，普通轮数等于领码次数时，普通额度为 `0`，该用户不能继续从普通队列向外提供内容。插队额度独立于普通额度，每投入 `1` 积分增加 `1` 次插队额度。

### 一键领码状态转移

`POST /api/v1/activity/use` 成功后，无论内容来自哪个队列，领取人都会获得一次领码记录和一次新的普通额度：

```text
领取人：
  used_count       + 1
  ordinary_credit  + 1
```

内容所属用户按实际命中的队列更新：

```text
命中普通队列：
  ordinary_rounds  + 1
  ordinary_credit  - 1

命中插队队列：
  boost_rounds     + 1
  priority_credit  - 1
```

领取人和内容所属用户的更新在同一个 MySQL 事务内完成，并对双方活动记录加行锁。这样并发领取不会让同一份额度被重复消费。普通队列还会在事务内再次验证：

```text
ordinary_credit > 0 AND ordinary_rounds < used_count
```

Redis 中即使短暂残留了过期 ID，也不能绕过数据库校验；过期 ID 会被移出队列，然后继续寻找下一个候选人。

### Redis 队列结构

每种活动类型分别维护普通队列和插队队列。每个队列由一个 List 和一个 Set 组成：

```text
activity_queue:{activityType}:ordinary:list
activity_queue:{activityType}:ordinary:members

activity_queue:{activityType}:priority:list
activity_queue:{activityType}:priority:members

activity_queue:{activityType}:seeded
```

- `list` 保存轮询顺序。领取时使用 Lua 执行 `LPOP` 后 `RPUSH`，形成循环队列。
- `members` 用于原子防重，保证一个用户在同类队列中最多出现一次。
- `seeded` 表示该活动类型的 Redis 队列已经根据 MySQL 初始化。
- 轮询会跳过领取人自己的 UID；队列为空或只有本人时返回空队列。
- 同一个用户可以同时拥有普通额度和插队额度，因此可以同时存在于两个队列。

普通队列入队条件：

```text
ordinary_credit > 0 AND ordinary_rounds < used_count
```

插队队列入队条件：

```text
priority_credit > 0
```

相应额度消费至 `0` 后，用户会从该队列的 List 和 Set 中同时删除。成功领码产生的新普通额度会让领取人进入普通队列。

### 候选内容选择顺序

领取流程固定为：

1. 检查领取人已经发布当前活动类型的内容。
2. 确保 Redis 队列已经从 MySQL 初始化。
3. 从插队队列循环选择第一个不是领取人自己的 UID。
4. 插队队列没有可用候选人时，再查询普通队列。
5. 在 MySQL 事务中重新校验并消费额度，返回候选人的活动内容。
6. 如果候选记录或额度已经失效，清理 Redis 项并继续查找。

插队队列内部和普通队列内部都是轮询，而不是按剩余额度排序。只要还存在其他用户的有效插队额度，新的领取请求就会优先消费插队队列；插队队列无有效候选人后才会消费普通队列。

### Redis 重建与一致性

API 启动时会清空所有活动类型的 Redis 活动队列和 `seeded` 标记。第一次领取时，再按 MySQL 中的实际剩余额度延迟重建队列。这意味着：

- MySQL 数据决定用户是否真正拥有可用额度。
- Redis 丢失或服务重启不会丢失轮数与额度。
- Redis 入队失败时会使 `seeded` 失效，后续请求可以重新构建队列。

### 领取按钮状态

活动详情和领取结果中的 `can_claim` 不依赖 Redis 队列长度，而是直接检查 MySQL 是否存在其他用户的有效内容：

```text
uid != 当前用户
AND (
  priority_credit > 0
  OR (ordinary_credit > 0 AND ordinary_rounds < used_count)
)
```

没有符合条件的其他用户时——包括队列为空或队列中只有本人——接口返回 `can_claim: false`，前端将“一键领码”按钮置灰。

### 主要实现位置

- `internal/platform/platform.go`：发布入队、插队积分、候选选择、事务更新、可领取状态判断。
- `internal/queue/activity.go`：Redis List/Set 键、Lua 原子防重、轮询、跳过本人和移除操作。
- `internal/domain/models.go`：活动轮数、累计次数和剩余额度字段。
- `internal/database/database.go`：历史轮数修正及普通额度重算迁移。
- `internal/platform/platform_test.go`：领取计数、额度转移、插队优先、跳过本人和额度耗尽回归测试。
- `internal/queue/activity_test.go`：Redis 队列轮询、防重、仅本人和重置测试。

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
