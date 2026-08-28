export type InviteActivityType =
  | 'buy_food'
  | 'cash_turntable'
  | 'cash_monopoly'
  | 'daily_cash'

export type ActivityIcon = 'grocery' | 'turntable' | 'monopoly' | 'daily'

export interface InviteActivityConfig {
  type: InviteActivityType
  title: string
  shortTitle: string
  path: string
  icon: ActivityIcon
  intro: string
  placeholder: string
  guideTitle: string
  guideSteps: string[]
}

export const activityConfigs: Record<InviteActivityType, InviteActivityConfig> = {
  buy_food: {
    type: 'buy_food',
    title: '买菜邀请',
    shortTitle: '买菜邀请',
    path: '/grocery-invite',
    icon: 'grocery',
    intro: '温馨提示：发布自己的邀请内容后，有一定的默认被他人领取次数，需要你一键领码帮助他人来获得额外的次数，或者使用积分。',
    placeholder: '微信返回后粘贴到这里',
    guideTitle: '如何获取邀请内容',
    guideSteps: ['首页', '点击「多多买菜」', '点击「邀请赚现金」', '再次点击「立即邀请」', '点击「去微信粘贴给好友」'],
  },
  cash_turntable: {
    type: 'cash_turntable',
    title: '现金大转盘',
    shortTitle: '现金大转盘',
    path: '/cash-turntable',
    icon: 'turntable',
    intro: '温馨提示：发布自己的邀请内容后，有一定的默认被他人领取次数，需要你一键领码帮助他人来获得额外的次数，或者使用积分。',
    placeholder: '粘贴现金大转盘邀请内容',
    guideTitle: '如何获取现金大转盘邀请内容',
    guideSteps: ['打开现金大转盘活动', '点击「邀请好友」', '选择「微信好友」', '返回浏览器', '将邀请内容粘贴到发布框'],
  },
  cash_monopoly: {
    type: 'cash_monopoly',
    title: '现金大富翁',
    shortTitle: '现金大富翁',
    path: '/cash-monopoly',
    icon: 'monopoly',
    intro: '温馨提示：发布自己的邀请内容后，有一定的默认被他人领取次数，需要你一键领码帮助他人来获得额外的次数，或者使用积分。',
    placeholder: '粘贴现金大富翁邀请内容',
    guideTitle: '如何获取现金大富翁邀请内容',
    guideSteps: ['打开现金大富翁活动', '点击「邀请好友」', '选择「微信好友」', '返回浏览器', '将邀请内容粘贴到发布框'],
  },
  daily_cash: {
    type: 'daily_cash',
    title: '天天领现金',
    shortTitle: '天天领现金',
    path: '/daily-cash',
    icon: 'daily',
    intro: '温馨提示：发布自己的邀请内容后，有一定的默认被他人领取次数，需要你一键领码帮助他人来获得额外的次数，或者使用积分。',
    placeholder: '粘贴天天领现金邀请内容',
    guideTitle: '如何获取天天领现金邀请内容',
    guideSteps: ['打开天天领现金活动', '点击「邀请好友」', '选择「微信好友」', '返回浏览器', '将邀请内容粘贴到发布框'],
  },
}

export const inviteActivities = Object.values(activityConfigs)

export interface LuckyTeamConfig {
  title: string
  intro: string
  placeholder: string
  inputLabel: string
  publishButton: string
  publishingButton: string
  guideButton: string
  guideTitle: string
  guideSteps: string[]
  guideNote: string
  listTitle: string
  refreshButton: string
  receiveButton: string
  useButton: string
  usingButton: string
  emptyText: string
  loadingLabel: string
  invalidCodeError: string
  publishedToast: string
  listRefreshedToast: string
  usedToast: string
  receivedToast: string
}

export const luckyTeamConfig: LuckyTeamConfig = {
  title: '福袋组队',
  intro: '温馨提示：请确保填写的福袋码真实有效。严禁发布虚假无效信息，否则将会被限流。',
  placeholder: '填写 8 或 9 位福袋码',
  inputLabel: '福袋码',
  publishButton: '立即发布',
  publishingButton: '处理中',
  guideButton: '如何获取邀请福袋码',
  guideTitle: '如何参与抽福袋',
  guideSteps: [
    '首页',
    '点击「百亿补贴」',
    '点击「抽福袋」',
    '再次点击「抽福袋」',
    '点击「邀请好友抽福袋」',
    '长按图片中的数字',
    '复制分词文本',
    '点击「复制」',
  ],
  guideNote: '图片分词文本的功能因手机品牌不同可能略有差异，比如魅族是单指长按，荣耀是双指长按。',
  listTitle: '可用福袋码',
  refreshButton: '刷新',
  receiveButton: '一键领码',
  useButton: '使用',
  usingButton: '领取中',
  emptyText: '当前没有可领取的福袋码',
  loadingLabel: '正在加载福袋码',
  invalidCodeError: '请输入 8 或 9 位数字福袋码',
  publishedToast: '福袋码发布成功',
  listRefreshedToast: '列表已刷新',
  usedToast: '福袋码已复制',
  receivedToast: '已领取并复制福袋码',
}
