const statusEl = document.getElementById('status')
const serverInput = document.getElementById('serverUrl')
const captureBtn = document.getElementById('captureBtn')

const setStatus = (msg) => {
  statusEl.textContent = msg
}

const loadServer = async () => {
  const data = await chrome.storage.sync.get(['serverUrl'])
  serverInput.value = data.serverUrl || 'http://localhost:8080'
}

const saveServer = async (value) => {
  await chrome.storage.sync.set({ serverUrl: value })
}

captureBtn.addEventListener('click', async () => {
  const serverUrl = serverInput.value.trim() || 'http://localhost:8080'
  await saveServer(serverUrl)
  setStatus('正在抓取页面...')

  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
  if (!tab || !tab.id) {
    setStatus('未找到活动标签页')
    return
  }

  chrome.tabs.sendMessage(tab.id, { type: 'capture' }, async (res) => {
    if (chrome.runtime.lastError) {
      setStatus('内容脚本未响应，请刷新页面')
      return
    }
    if (!res || !res.ok) {
      setStatus(res?.error || '抓取失败')
      return
    }

    try {
      const response = await fetch(`${serverUrl}/api/archives`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(res.payload),
      })
      if (!response.ok) throw new Error('后端保存失败')
      setStatus('归档成功')
    } catch (err) {
      setStatus(err.message || '请求失败')
    }
  })
})

loadServer()
