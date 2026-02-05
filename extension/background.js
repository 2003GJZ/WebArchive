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
  if (!response.ok) throw new Error('后端保存失败')
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
  chrome.runtime.sendMessage({ type: 'capture-status', status, message })
}

const handleCapture = async (mode, options) => {
  const { serverUrl, category, tags, autoTag, enableOptional } = options
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
  if (!tab || !tab.id) {
    setBadge('ERR', '#b00020')
    reportStatus('error', '未找到活动标签页')
    return
  }

  chrome.tabs.sendMessage(tab.id, { type: mode }, async (res) => {
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
        category: enableOptional ? (category || '') : '',
        tags: enableOptional ? (tags || []) : [],
        autoTag: Boolean(autoTag),
      }
      await postArchive(serverUrl, payload)
      setBadge('OK', '#2ea043')
      reportStatus('ok', '归档成功')
    } catch (_err) {
      setBadge('ERR', '#b00020')
      reportStatus('error', '请求失败')
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
