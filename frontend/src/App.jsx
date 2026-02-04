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
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    const fetchData = async () => {
      setLoading(true)
      setError('')
      try {
        const res = await fetch(`${API_BASE}/api/archives`)
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
  }, [])

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
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索标题 / 站点 / URL"
            />
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
