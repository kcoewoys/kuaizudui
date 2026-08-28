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
