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

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || msg.type !== 'capture') return

  try {
    const documentClone = document.cloneNode(true)
    const reader = new Readability(documentClone)
    const article = reader.parse()

    const payload = {
      url: location.href,
      title: article?.title || document.title || '',
      html: article?.content || document.documentElement.outerHTML,
      content: article?.textContent || '',
      excerpt: article?.excerpt || '',
      byline: article?.byline || '',
      siteName: article?.siteName || '',
      favicon: findFavicon(),
      capturedAt: new Date().toISOString(),
    }

    sendResponse({ ok: true, payload })
  } catch (err) {
    sendResponse({ ok: false, error: err.message || 'capture failed' })
  }
})
