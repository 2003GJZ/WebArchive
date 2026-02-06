const setBadge = (text, color) => {
  chrome.action.setBadgeText({ text })
  chrome.action.setBadgeBackgroundColor({ color })
  if (text) {
    setTimeout(() => chrome.action.setBadgeText({ text: '' }), 4000)
  }
}

const postArchive = async (serverUrl, payload) => {
  const response = await fetch(`${serverUrl}/api/archives`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  let data = null
  try {
    data = await response.json()
  } catch (_err) {
    data = null
  }
  if (!response.ok) {
    const msg = data?.error || '后端保存失败'
    throw new Error(msg)
  }
  return data
}

const requestAiTag = async (serverUrl, archiveId) => {
  const response = await fetch(`${serverUrl}/api/archives/${archiveId}/ai-tag`, {
    method: 'POST',
  })
  let data = null
  try {
    data = await response.json()
  } catch (_err) {
    data = null
  }
  if (!response.ok) {
    let msg = data?.error || 'AI 分类失败'
    if (msg.toLowerCase().includes('llm not configured')) {
      msg = 'AI 未配置，请在前端“AI 设置”里配置'
    }
    throw new Error(msg)
  }
  return data
}

const notifyPopup = (payload) => {
  try {
    chrome.runtime.sendMessage(payload, () => {
      if (chrome.runtime.lastError) {
        // popup may be closed; ignore
      }
    })
  } catch (_err) {
    // ignore sendMessage errors
  }
}

const reportStatus = async (status, message) => {
  try {
    await chrome.storage.sync.set({
      lastStatus: message,
      lastStatusAt: Date.now(),
    })
  } catch (_err) {
    // ignore storage errors
  }
  notifyPopup({ type: 'capture-status', status, message })
}

const handleCapture = async (mode, options) => {
  const { serverUrl, category, tags, autoTag, cleanContent } = options
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
  if (!tab || !tab.id) {
    setBadge('ERR', '#b00020')
    reportStatus('error', '未找到活动标签页')
    return
  }

  chrome.tabs.sendMessage(tab.id, { type: mode, cleanContent }, async (res) => {
    if (chrome.runtime.lastError) {
      setBadge('ERR', '#b00020')
      reportStatus('error', '内容脚本未响应，请刷新页面')
      return
    }
    if (!res || !res.ok) {
      setBadge('ERR', '#b00020')
      reportStatus('error', res?.error || '抓取失败')
      return
    }
    try {
      const payload = {
        ...res.payload,
        category: category || '',
        tags: tags || [],
        autoTag: Boolean(autoTag),
      }
      const archive = await postArchive(serverUrl, payload)
      if (autoTag && archive?.id) {
        reportStatus('progress', '已保存，AI 分类中…')
        await requestAiTag(serverUrl, archive.id)
      }
      setBadge('OK', '#2ea043')
      reportStatus('ok', '归档成功')
    } catch (err) {
      setBadge('ERR', '#b00020')
      reportStatus('error', err?.message || '请求失败')
    }
  })
}

chrome.runtime.onMessage.addListener((msg, _sender, _sendResponse) => {
  if (!msg || !msg.type) return
  if (msg.type === 'start-capture') {
    handleCapture('capture', msg)
  }
  if (msg.type === 'start-select') {
    handleCapture('select', msg)
  }
})
