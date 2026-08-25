# 前端
移动端前端: Vue 3、TypeScript、Tailwind CSS


## 运行

先启动真实后端及 MySQL、Redis：

```bash
cd backend
docker compose up --build
```

再打开另一个终端启动前端：

```bash
cd frontend
npm run dev
```

开发服务器会把 `/api` 代理到 `http://127.0.0.1:8080`。如果前后端分域部署，复制 `frontend/.env.example` 为 `frontend/.env`，并通过 `VITE_API_BASE_URL` 指定完整接口地址。

生产构建：

```bash
cd frontend
npm run build
```

## 页面

- `/` — 活动列表
- `/lucky-team` — 福袋组队
- `/grocery-invite` — 买菜邀请
- `/cash-turntable` — 现金大转盘
- `/cash-monopoly` — 现金大富翁
- `/daily-cash` — 天天领现金
- `/profile` — 个人中心、积分来源与兑换码弹窗
- `/?ref=手机号` — 邀请链接；访问后记录邀请关系，个人中心绑定手机号后可生成
- `/admin` — 手机端管理后台；进入时使用后端配置的管理员手机号验证

前端使用 History 路由，地址中不包含 `#`。开发服务器已自动支持子路径回退；使用 Nginx 部署时可直接采用 `frontend/nginx.conf`，确保刷新 `/admin` 等页面仍返回前端 `index.html`。

买菜邀请和三个现金活动共用 `InviteActivityPage.vue` 与 `useInviteActivity.ts`。活动差异只存在于 `frontend/src/domain/activities.ts` 的配置中，发布、200 字校验、排队、积分加速和复制逻辑保持一致。

前端已通过 `frontend/src/services/api.ts` 接入真实 Go 接口。业务数据全部存入 MySQL/Redis；浏览器只在 `localStorage` 保存后端签发的随机 UID，用于保持匿名用户会话。

# 后端

后端采用 Go、Gin、Gorm、MySQL 和 Redis，包含配置文件、Docker Compose、自动数据库迁移、Redis Lua 福袋队列、用户活动、积分兑换及管理员接口。运行说明、完整接口表和数据存储设计（每张 MySQL 表的用途、全部 Redis 键与每日重置）见 [`backend/README.md`](backend/README.md)。

管理员手机号配置在 `backend/config/config.yaml` 的 `business.admin_phone`。部署环境也可通过 `APP_ADMIN_PHONE` 覆盖；正确号码不会写入前端代码。
