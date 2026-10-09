export function openPinduoduoApp() {
  window.location.href = 'pinduoduo://'
}

// 从粘贴的邀请内容中提取链接并访问；返回是否找到链接，供调用方决定回退行为。
export function openContentLink(content: string): boolean {
  const match = content.match(/(?:https?|pinduoduo):\/\/\S+/i)
  if (!match) return false
  window.location.href = match[0].replace(/[)\]）>」』"”']+$/g, '')
  return true
}
