import React, { useEffect, useMemo, useState } from 'react'

const API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8080'

const formatDate = (value) => {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString('zh-CN')
}

export default function App() {
  const [items, setItems] = useState([])
  const [selected, setSelected] = useState(null)
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState('')
  const [tag, setTag] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [form, setForm] = useState({ category: '', tags: '' })
  const [saving, setSaving] = useState(false)

  const loadArchives = async () => {
    const fetchData = async () => {
      setLoading(true)
      setError('')
      try {
        const params = new URLSearchParams()
        if (query) params.set('q', query)
        if (category) params.set('category', category)
        if (tag) params.set('tag', tag)
        const qs = params.toString()
        const res = await fetch(`${API_BASE}/api/archives${qs ? `?${qs}` : ''}`)
        if (!res.ok) throw new Error('加载失败')
        const data = await res.json()
        setItems(data)
        if (data.length > 0) setSelected(data[0])
      } catch (err) {
        setError(err.message || '加载失败')
      } finally {
        setLoading(false)
      }
    }
    fetchData()
  }

  useEffect(() => {
    loadArchives()
  }, [])

  useEffect(() => {
    if (!selected) {
      setForm({ category: '', tags: '' })
      return
    }
    let tags = []
    if (Array.isArray(selected.tags)) tags = selected.tags
    setForm({ category: selected.category || '', tags: tags.join(', ') })
  }, [selected])

  const filtered = useMemo(() => {
    if (!query) return items
    const q = query.toLowerCase()
    return items.filter((item) =>
      [item.title, item.siteName, item.url].some((v) => (v || '').toLowerCase().includes(q))
    )
  }, [items, query])

  const stats = useMemo(() => {
    return {
      total: items.length,
      latest: items[0]?.createdAt ? formatDate(items[0].createdAt) : '暂无',
    }
  }, [items])

  return (
    <div className="app">
      <header className="hero">
        <div>
          <p className="eyebrow">WebArchive</p>
          <h1>把有价值的网页，变成自己的离线知识库</h1>
          <p className="subtext">
            插件一键采集，后端自动归档，前端沉浸式预览。当前使用单用户模式，兼容 Edge。
          </p>
        </div>
        <div className="stats">
          <div>
            <span className="label">归档数量</span>
            <strong>{stats.total}</strong>
          </div>
          <div>
            <span className="label">最近更新</span>
            <strong>{stats.latest}</strong>
          </div>
        </div>
      </header>

      <main className="layout">
        <section className="panel list-panel">
          <div className="panel-header">
            <h2>归档列表</h2>
            <button type="button" className="ghost" onClick={loadArchives}>
              刷新
            </button>
          </div>
          <div className="filters">
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索标题 / 站点 / 正文"
            />
            <input
              value={category}
              onChange={(e) => setCategory(e.target.value)}
              placeholder="分类"
            />
            <input value={tag} onChange={(e) => setTag(e.target.value)} placeholder="标签" />
            <button type="button" className="primary" onClick={loadArchives}>
              搜索
            </button>
          </div>
          {loading && <div className="hint">加载中…</div>}
          {error && <div className="error">{error}</div>}
          <div className="list">
            {filtered.map((item) => (
              <button
                type="button"
                key={item.id}
                className={`card ${selected?.id === item.id ? 'active' : ''}`}
                onClick={() => setSelected(item)}
              >
                <div className="card-title">{item.title || '未命名页面'}</div>
                <div className="card-meta">
                  <span>{item.siteName || '未知站点'}</span>
                  <span>{formatDate(item.createdAt)}</span>
                </div>
                <div className="card-url">{item.url}</div>
              </button>
            ))}
            {!loading && filtered.length === 0 && <div className="hint">暂无归档内容</div>}
          </div>
        </section>

        <section className="panel preview-panel">
          <div className="panel-header">
            <h2>内容预览</h2>
            {selected && (
              <a href={selected.url} target="_blank" rel="noreferrer">
                打开原文
              </a>
            )}
          </div>
          {selected && (
            <div className="meta-panel">
              <div>
                <label>分类</label>
                <input
                  value={form.category}
                  onChange={(e) => setForm((s) => ({ ...s, category: e.target.value }))}
                  placeholder="例如：技术/产品"
                />
              </div>
              <div>
                <label>标签</label>
                <input
                  value={form.tags}
                  onChange={(e) => setForm((s) => ({ ...s, tags: e.target.value }))}
                  placeholder="用逗号分隔"
                />
              </div>
              <button
                type="button"
                className="primary"
                disabled={!selected || saving}
                onClick={async () => {
                  if (!selected) return
                  setSaving(true)
                  try {
                    const tags = form.tags
                      .split(',')
                      .map((t) => t.trim())
                      .filter(Boolean)
                    const res = await fetch(`${API_BASE}/api/archives/${selected.id}`, {
                      method: 'PATCH',
                      headers: { 'Content-Type': 'application/json' },
                      body: JSON.stringify({ category: form.category, tags }),
                    })
                    if (!res.ok) throw new Error('保存失败')
                    await loadArchives()
                  } catch (err) {
                    setError(err.message || '保存失败')
                  } finally {
                    setSaving(false)
                  }
                }}
              >
                {saving ? '保存中…' : '保存元信息'}
              </button>
            </div>
          )}
          {!selected && <div className="hint">选择左侧内容即可预览</div>}
          {selected && (
            <iframe
              title="archive-preview"
              src={`${API_BASE}/api/archives/${selected.id}/html`}
            />
          )}
        </section>
      </main>
    </div>
  )
}
