const statusEl = document.getElementById('status')
const serverInput = document.getElementById('serverUrl')
const captureBtn = document.getElementById('captureBtn')
const selectBtn = document.getElementById('selectBtn')
const categoryInput = document.getElementById('category')
const tagsInput = document.getElementById('tags')
const autoTagInput = document.getElementById('autoTag')
const enableOptionalInput = document.getElementById('enableOptional')
const optionalDetails = document.getElementById('optionalDetails')

const setStatus = (msg) => {
  statusEl.textContent = msg
}

const updateOptionalState = () => {
  const enabled = enableOptionalInput?.checked
  if (optionalDetails) {
    if (enabled) {
      optionalDetails.classList.remove('disabled')
    } else {
      optionalDetails.classList.add('disabled')
      optionalDetails.removeAttribute('open')
    }
  }
}

const loadServer = async () => {
  const data = await chrome.storage.sync.get([
    'serverUrl',
    'category',
    'tags',
    'autoTag',
    'enableOptional',
    'lastStatus',
  ])
  serverInput.value = data.serverUrl || 'http://localhost:8080'
  if (categoryInput) categoryInput.value = data.category || ''
  if (tagsInput) tagsInput.value = data.tags || ''
  if (autoTagInput) autoTagInput.checked = Boolean(data.autoTag)
  if (enableOptionalInput) {
    enableOptionalInput.checked = Boolean(data.enableOptional)
    updateOptionalState()
  }
  if (data.lastStatus) setStatus(data.lastStatus)
}

const saveSettings = async () => {
  await chrome.storage.sync.set({
    serverUrl: serverInput.value.trim(),
    category: categoryInput?.value.trim() || '',
    tags: tagsInput?.value.trim() || '',
    autoTag: Boolean(autoTagInput?.checked),
    enableOptional: Boolean(enableOptionalInput?.checked),
  })
}

const buildTags = () => {
  if (!enableOptionalInput?.checked) return []
  return (tagsInput?.value || '')
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean)
}

const getCategory = () => {
  if (!enableOptionalInput?.checked) return ''
  return categoryInput?.value.trim() || ''
}

const sendToBackground = async (type) => {
  await saveSettings()
  captureBtn.disabled = true
  selectBtn.disabled = true
  chrome.runtime.sendMessage({
    type,
    serverUrl: serverInput.value.trim() || 'http://localhost:8080',
    category: getCategory(),
    tags: buildTags(),
    autoTag: Boolean(autoTagInput?.checked),
    enableOptional: Boolean(enableOptionalInput?.checked),
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

if (enableOptionalInput) {
  enableOptionalInput.addEventListener('change', () => {
    updateOptionalState()
    saveSettings()
  })
}

chrome.runtime.onMessage.addListener((msg) => {
  if (!msg || msg.type !== 'capture-status') return
  setStatus(msg.message || '')
  captureBtn.disabled = false
  selectBtn.disabled = false
})

loadServer()
