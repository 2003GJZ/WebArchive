const findFavicon = () => {
  const selectors = [
    'link[rel="icon"]',
    'link[rel="shortcut icon"]',
    'link[rel="apple-touch-icon"]',
  ]
  for (const sel of selectors) {
    const el = document.querySelector(sel)
    if (el && el.href) return el.href
  }
  return ''
}

const captureReadable = () => {
  const documentClone = document.cloneNode(true)
  const reader = new Readability(documentClone)
  const article = reader.parse()
  return {
    title: article?.title || document.title || '',
    html: article?.content || document.documentElement.outerHTML,
    content: article?.textContent || '',
    excerpt: article?.excerpt || '',
    byline: article?.byline || '',
    siteName: article?.siteName || '',
  }
}

const buildPayload = (html, text, meta = {}) => ({
  url: location.href,
  title: meta.title || document.title || '',
  html: html || document.documentElement.outerHTML,
  content: text || '',
  excerpt: meta.excerpt || '',
  byline: meta.byline || '',
  siteName: meta.siteName || '',
  favicon: findFavicon(),
  capturedAt: new Date().toISOString(),
})

const createOverlay = () => {
  const overlay = document.createElement('div')
  overlay.style.position = 'fixed'
  overlay.style.zIndex = '2147483647'
  overlay.style.pointerEvents = 'none'
  overlay.style.border = '2px solid #ff6a3d'
  overlay.style.background = 'rgba(255, 106, 61, 0.1)'
  document.body.appendChild(overlay)
  return overlay
}

const startSelection = (sendResponse) => {
  const overlay = createOverlay()
  let active = true

  const cleanup = () => {
    active = false
    overlay.remove()
    document.removeEventListener('mousemove', onMove, true)
    document.removeEventListener('click', onClick, true)
    document.removeEventListener('keydown', onKey, true)
  }

  const onMove = (evt) => {
    if (!active) return
    const el = evt.target
    if (!el || !(el instanceof HTMLElement)) return
    const rect = el.getBoundingClientRect()
    overlay.style.left = `${rect.left}px`
    overlay.style.top = `${rect.top}px`
    overlay.style.width = `${rect.width}px`
    overlay.style.height = `${rect.height}px`
  }

  const onClick = (evt) => {
    if (!active) return
    evt.preventDefault()
    evt.stopPropagation()
    const el = evt.target
    cleanup()
    if (!el || !(el instanceof HTMLElement)) {
      sendResponse({ ok: false, error: '选区无效' })
      return
    }
    const payload = buildPayload(el.outerHTML, el.textContent || '')
    sendResponse({ ok: true, payload })
  }

  const onKey = (evt) => {
    if (evt.key === 'Escape') {
      cleanup()
      sendResponse({ ok: false, error: '已取消选区' })
    }
  }

  document.addEventListener('mousemove', onMove, true)
  document.addEventListener('click', onClick, true)
  document.addEventListener('keydown', onKey, true)
}

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg) return

  if (msg.type === 'capture') {
    try {
      const article = captureReadable()
      const payload = buildPayload(article.html, article.content, article)
      sendResponse({ ok: true, payload })
    } catch (err) {
      sendResponse({ ok: false, error: err.message || 'capture failed' })
    }
    return
  }

  if (msg.type === 'select') {
    startSelection(sendResponse)
    return true
  }
})
