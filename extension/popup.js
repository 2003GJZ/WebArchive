const statusEl = document.getElementById('status')
const serverInput = document.getElementById('serverUrl')
const captureBtn = document.getElementById('captureBtn')
const selectBtn = document.getElementById('selectBtn')
const categoryInput = document.getElementById('category')
const tagsInput = document.getElementById('tags')
const autoTagInput = document.getElementById('autoTag')

const setStatus = (msg) => {
  statusEl.textContent = msg
}

const loadServer = async () => {
  const data = await chrome.storage.sync.get(['serverUrl', 'category', 'tags', 'autoTag', 'lastStatus'])
  serverInput.value = data.serverUrl || 'http://localhost:8080'
  if (categoryInput) categoryInput.value = data.category || ''
  if (tagsInput) tagsInput.value = data.tags || ''
  if (autoTagInput) autoTagInput.checked = Boolean(data.autoTag)
  if (data.lastStatus) setStatus(data.lastStatus)
}

const saveSettings = async () => {
  await chrome.storage.sync.set({
    serverUrl: serverInput.value.trim(),
    category: categoryInput?.value.trim() || '',
    tags: tagsInput?.value.trim() || '',
    autoTag: Boolean(autoTagInput?.checked),
  })
}

const buildTags = () =>
  (tagsInput?.value || '')
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean)

const sendToBackground = async (type) => {
  await saveSettings()
  captureBtn.disabled = true
  selectBtn.disabled = true
  chrome.runtime.sendMessage({
    type,
    serverUrl: serverInput.value.trim() || 'http://localhost:8080',
    category: categoryInput?.value.trim() || '',
    tags: buildTags(),
    autoTag: Boolean(autoTagInput?.checked),
  })
}

captureBtn.addEventListener('click', async () => {
  setStatus('开始抓取，请稍候…')
  await sendToBackground('start-capture')
})

selectBtn.addEventListener('click', async () => {
  setStatus('进入选区模式，点击页面元素')
  await sendToBackground('start-select')
})

chrome.runtime.onMessage.addListener((msg) => {
  if (!msg || msg.type !== 'capture-status') return
  setStatus(msg.message || '')
  captureBtn.disabled = false
  selectBtn.disabled = false
})

loadServer()
