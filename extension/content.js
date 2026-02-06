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

const escapeHtml = (value) =>
  String(value || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')

const collectInlineSheetRules = () => {
  const rules = []
  const sheets = Array.from(document.styleSheets || [])
  sheets.forEach((sheet) => {
    try {
      if (!sheet.cssRules) return
      for (const rule of sheet.cssRules) {
        if (rule && rule.cssText) rules.push(rule.cssText)
      }
    } catch (_err) {
      // cross-origin stylesheets are not accessible
    }
  })
  return rules.join('\n')
}

const collectHeadHTML = () => {
  const parts = ['<meta charset="utf-8" />', `<title>${escapeHtml(document.title || '')}</title>`]
  document.querySelectorAll('link[rel="stylesheet"]').forEach((link) => {
    if (link.href) {
      parts.push(`<link rel="stylesheet" href="${link.href}" />`)
    }
  })
  document.querySelectorAll('style').forEach((style) => {
    if (style.textContent) {
      parts.push(`<style>${style.textContent}</style>`)
    }
  })
  const inlineRules = collectInlineSheetRules()
  if (inlineRules) {
    parts.push(`<style>${inlineRules}</style>`)
  }
  return parts.join('\n')
}

const getBodyAttrs = () => ({
  id: document.body?.id || '',
  className: document.body?.className || '',
})

const findWrapperElement = () => {
  const preferredIds = ['app', 'root', '__next', '__nuxt', 'main', 'content']
  for (const id of preferredIds) {
    const el = document.getElementById(id)
    if (el) return el
  }
  const body = document.body
  if (!body) return null
  const candidates = Array.from(body.children).filter((node) => node.tagName && node.tagName !== 'SCRIPT')
  if (candidates.length === 1) return candidates[0]
  return null
}

const wrapWithContainer = (innerHTML) => {
  const wrapper = findWrapperElement()
  if (!wrapper) {
    return `<div class="webarchive-root">${innerHTML}</div>`
  }
  const tag = wrapper.tagName.toLowerCase()
  const attrs = []
  if (wrapper.id) attrs.push(`id="${escapeHtml(wrapper.id)}"`)
  if (wrapper.className) attrs.push(`class="${escapeHtml(wrapper.className)}"`)
  const attrText = attrs.length > 0 ? ` ${attrs.join(' ')}` : ''
  return `<${tag}${attrText}>${innerHTML}</${tag}>`
}

const wrapHtmlDocument = (bodyHTML, opts = {}) => {
  const attrs = []
  if (opts.bodyId) attrs.push(`id="${escapeHtml(opts.bodyId)}"`)
  if (opts.bodyClass) attrs.push(`class="${escapeHtml(opts.bodyClass)}"`)
  const wrapped = opts.wrap ? wrapWithContainer(bodyHTML) : bodyHTML
  const attrText = attrs.length > 0 ? ` ${attrs.join(' ')}` : ''
  return `<!doctype html><html><head>${collectHeadHTML()}</head><body${attrText}>${wrapped}</body></html>`
}

const captureReadable = () => {
  const documentClone = document.cloneNode(true)
  const reader = new Readability(documentClone)
  const article = reader.parse()
  const bodyHTML = article?.content || document.body.innerHTML || document.documentElement.outerHTML
  const bodyAttrs = getBodyAttrs()
  return {
    title: article?.title || document.title || '',
    html: wrapHtmlDocument(bodyHTML, { wrap: true, bodyId: bodyAttrs.id, bodyClass: bodyAttrs.className }),
    content: article?.textContent || document.body.textContent || '',
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
  overlay.style.border = '2px solid #1f6feb'
  overlay.style.background = 'rgba(31, 111, 235, 0.12)'
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
    const bodyAttrs = getBodyAttrs()
    const html = wrapHtmlDocument(el.outerHTML, { wrap: true, bodyId: bodyAttrs.id, bodyClass: bodyAttrs.className })
    const payload = buildPayload(html, el.textContent || '')
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
      let payload
      if (msg.cleanContent !== false) {
        const article = captureReadable()
        payload = buildPayload(article.html, article.content, article)
      } else {
        const bodyAttrs = getBodyAttrs()
        const html = wrapHtmlDocument(document.body.innerHTML || document.documentElement.outerHTML, {
          wrap: false,
          bodyId: bodyAttrs.id,
          bodyClass: bodyAttrs.className,
        })
        payload = buildPayload(html, document.body.textContent || '', { title: document.title })
      }
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
