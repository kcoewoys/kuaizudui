# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- 普通用户通过手机浏览器发布、领取和复制福袋、买菜及现金活动内容，并管理手机号、积分与兑换码。
- 运营管理员通过管理后台充值积分、查看充值记录、生成兑换码以及维护公告和交流群二维码。

## Product Purpose

eaok.cn 是一个移动端优先的短内容分享与积分运营平台。成功意味着用户能快速完成发布和领取，运营人员能在同一后台安全、清晰地维护用户和活动配置。

## Positioning

平台用匿名 UID、手机号绑定、Redis FIFO 福袋队列与统一的活动内容模型，把多个现金及邀请玩法收敛为一致的发布、领取和运营流程。

## Operating Context

- 用户端主要在手机浏览器使用。
- 管理端仅在手机浏览器通过 `/admin` 进入；进入时弹出手机号验证，不设置独立登录页或密码字段。
- 管理员手机号仅由后端配置文件或环境变量维护。前端不得包含、返回或暗示正确号码。
- 验证成功后，后端签发短期管理会话令牌，用于保护后续管理接口；错误手机号不能看到后台数据。

## Capabilities and Constraints

- 前端：Vue 3、TypeScript、Tailwind CSS、统一 API 服务。
- 后端：Go、Gin、Gorm、MySQL、Redis。
- 管理端覆盖积分充值与充值记录、首页公告、兑换码生成与状态查询、交流群二维码图片上传，以及用户反馈查看。
- 活动内容最长 200 字，福袋码为 8–9 位数字；管理员配置不能绕过后端校验。
- 管理员手机号配置项为 `business.admin_phone`，可由 `APP_ADMIN_PHONE` 覆盖。
- 二维码由后端接收 PNG/JPEG 文件并持久化，前端不要求管理员填写图片地址。

## Brand Commitments

- 产品名称与域名为 eaok.cn。
- 用户端视觉以 Google Stitch `fudai-v2` 为依据；新增管理端延续同一绿色交互色、蓝白画布和清晰克制的工具型表达。

## Evidence on Hand

- 产品需求：`/Users/sequoia/Downloads/短内容分享平台 PRD 产品需求文档.md`
- Stitch 原始页面与截图：`stitch-export/`
- 已实现的真实接口：`../backend/internal/httpapi/router.go`
- 现有设计系统：`DESIGN.md`
- 不得虚构商业数据、客户案例或运营成效。

## Product Principles

- 后端是身份、权限和业务数据的唯一可信来源。
- 高频操作必须能快速扫描、确认结果并恢复错误。
- 用户端的四种邀请活动共享一致逻辑，避免玩法之间产生行为分叉。
- 管理操作必须显示明确的进行中、成功、失败和空状态。
- 正确的管理员手机号永远不进入前端代码或提示文案。

## Accessibility & Inclusion

- 所有表单提供可见标签、键盘焦点和文字错误提示。
- 状态不能只依赖颜色表达；管理端以 320–480 px 手机视口和触控操作为交付范围。
