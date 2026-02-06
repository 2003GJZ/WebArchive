import React, { useEffect, useMemo, useRef, useState } from 'react'

const API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8080'

const formatDateTime = (value) => {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString('zh-CN')
}

const toTagList = (value) => {
  if (Array.isArray(value)) return value
  if (typeof value === 'string') {
    return value
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean)
  }
  return []
}

const splitHierarchyPaths = (value) =>
  value
    .split(/\n|;/)
    .map((line) => line.trim())
    .filter(Boolean)

const GraphView = ({ data, onNodeClick }) => {
  const wrapRef = useRef(null)
  const graphRef = useRef(null)

  useEffect(() => {
    let disposed = false
    const init = async () => {
      const [{ default: ForceGraph3D }, { default: SpriteText }, { default: THREE }] =
        await Promise.all([import('3d-force-graph'), import('three-spritetext'), import('three')])
      if (disposed || !wrapRef.current) return
      if (!graphRef.current) {
        const colors = {
          archive: '#6f8df2',
          category: '#f0a46b',
          tag: '#e9d5c6',
          path: '#b1c3ff',
          taxonomy: '#7ab7ff',
        }
        const sizes = {
          archive: 6,
          category: 5,
          tag: 3.8,
          path: 3.5,
          taxonomy: 4.8,
        }
        graphRef.current = ForceGraph3D()(wrapRef.current)
          .backgroundColor('#f7f8fb')
          .nodeLabel('label')
          .linkOpacity(0.35)
          .linkWidth((link) => (link.value || 1) * 0.7)
          .nodeThreeObject((node) => {
            const group = new THREE.Group()
            const radius = sizes[node.group] || 4
            const material = new THREE.MeshStandardMaterial({
              color: colors[node.group] || '#cbd2dd',
              roughness: 0.35,
              metalness: 0.05,
            })
            const sphere = new THREE.Mesh(new THREE.SphereGeometry(radius, 24, 24), material)
            const sprite = new SpriteText(node.label || '')
            sprite.color = node.group === 'archive' ? '#1f1f23' : '#5a5f6a'
            sprite.textHeight = node.group === 'archive' ? 9 : 6
            sprite.position.set(0, radius + 2, 0)
            group.add(sphere)
            group.add(sprite)
            return group
          })
          .onNodeClick((node) => onNodeClick?.(node))
        if (!graphRef.current.__lightsAdded) {
          graphRef.current.__lightsAdded = true
          graphRef.current.scene().add(new THREE.AmbientLight(0xffffff, 0.78))
          const dirLight = new THREE.DirectionalLight(0xffffff, 0.68)
          dirLight.position.set(30, 60, 20)
          graphRef.current.scene().add(dirLight)
        }
      }
      graphRef.current.graphData(data || { nodes: [], links: [] })
    }
    init()
    return () => {
      disposed = true
    }
  }, [data, onNodeClick])

  return <div className="graph-canvas" ref={wrapRef} />
}

const buildFocusGraph = (item) => {
  if (!item) return { nodes: [], links: [] }
  const nodes = []
  const links = []
  const seen = new Set()

  const addNode = (id, label, group, refId) => {
    if (seen.has(id)) return
    seen.add(id)
    nodes.push({ id, label, group, refId })
  }
  const addLink = (source, target, value = 1) => {
    if (!source || !target) return
    links.push({ source, target, value })
  }

  const centerId = `arc:${item.id}`
  addNode(centerId, item.title || item.url || '未命名页面', 'archive', item.id)

  if (item.category) {
    const catId = `cat:${item.category}`
    addNode(catId, item.category, 'category')
    addLink(catId, centerId, 2)
  }

  const tags = toTagList(item.tags)
  tags.forEach((t) => {
    const tagId = `tag:${t}`
    addNode(tagId, t, 'tag')
    addLink(tagId, centerId, 1)
  })

  const path = Array.isArray(item.hierarchy) ? item.hierarchy : []
  if (path.length > 0) {
    let prev = ''
    path.forEach((p, idx) => {
      const id = `path:${path.slice(0, idx + 1).join('/')}`
      addNode(id, p, 'path')
      if (prev) addLink(prev, id, 1)
      prev = id
    })
    if (prev) addLink(prev, centerId, 1)
  }

  return { nodes, links }
}

const buildTaxonomyGraph = (tree) => {
  if (!Array.isArray(tree)) return { nodes: [], links: [] }
  const nodes = []
  const links = []
  const add = (node, parentId) => {
    nodes.push({
      id: `tax:${node.id}`,
      label: node.label,
      group: 'taxonomy',
      refId: node.id,
    })
    if (parentId) {
      links.push({ source: parentId, target: `tax:${node.id}`, value: 2 })
    }
    if (Array.isArray(node.children)) {
      node.children.forEach((child) => add(child, `tax:${node.id}`))
    }
  }
  tree.forEach((root) => add(root, null))
  return { nodes, links }
}

export default function App() {
  const [items, setItems] = useState([])
  const [selected, setSelected] = useState(null)
  const [view, setView] = useState('list')
  const [graphMode, setGraphMode] = useState('taxonomy')
  const [graphData, setGraphData] = useState(null)
  const [taxonomyTree, setTaxonomyTree] = useState([])
  const [taxonomyDetail, setTaxonomyDetail] = useState(null)
  const [taxonomyLoading, setTaxonomyLoading] = useState(false)
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState('')
  const [tag, setTag] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [form, setForm] = useState({ category: '', tags: '', hierarchy: '' })
  const [saving, setSaving] = useState(false)
  const [metaOpen, setMetaOpen] = useState(false)
  const [aiLoading, setAiLoading] = useState(false)
  const [aiOpen, setAiOpen] = useState(false)
  const [selectedIds, setSelectedIds] = useState([])
  const [batchMode, setBatchMode] = useState(false)
  const [immersiveMode, setImmersiveMode] = useState(false)
  const [analysis, setAnalysis] = useState({
    running: false,
    lastRun: null,
    lastError: '',
    loopCount: 0,
    lastLoopScanned: 0,
    lastLoopProcessed: 0,
    totalProcessed: 0,
  })
  const [analysisLoading, setAnalysisLoading] = useState(false)
  const [aiConfig, setAiConfig] = useState({
    baseUrl: localStorage.getItem('aiBaseUrl') || 'https://api.openai.com/v1',
    model: localStorage.getItem('aiModel') || '',
    apiKey: localStorage.getItem('aiKey') || '',
  })

  const loadArchives = async () => {
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
      setSelectedIds((prev) => prev.filter((id) => data.some((item) => item.id === id)))
      if (data.length === 0) {
        setSelected(null)
      } else if (!selected || !data.find((item) => item.id === selected.id)) {
        setSelected(data[0])
      }
    } catch (err) {
      setError(err.message || '加载失败')
    } finally {
      setLoading(false)
    }
  }

  const loadGraph = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/graph`)
      if (!res.ok) throw new Error('图谱加载失败')
      const data = await res.json()
      setGraphData(data)
    } catch (err) {
      setError(err.message || '图谱加载失败')
    }
  }

  const loadTaxonomy = async () => {
    setTaxonomyLoading(true)
    setTaxonomyDetail(null)
    try {
      const res = await fetch(`${API_BASE}/api/taxonomy`)
      if (!res.ok) throw new Error('分类树加载失败')
      const data = await res.json()
      setTaxonomyTree(data)
    } catch (err) {
      setError(err.message || '分类树加载失败')
    } finally {
      setTaxonomyLoading(false)
    }
  }

  const loadTaxonomyNode = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/api/taxonomy/${id}?desc=1`)
      if (!res.ok) throw new Error('节点加载失败')
      const data = await res.json()
      setTaxonomyDetail(data)
    } catch (err) {
      setError(err.message || '节点加载失败')
    }
  }

  const loadAnalysisStatus = async (silent = false) => {
    if (!silent) setAnalysisLoading(true)
    try {
      const res = await fetch(`${API_BASE}/api/ai/analyze/status`)
      if (!res.ok) throw new Error('后台分析状态获取失败')
      const data = await res.json()
      setAnalysis(data)
    } catch (err) {
      if (!silent) setError(err.message || '后台分析状态获取失败')
    } finally {
      if (!silent) setAnalysisLoading(false)
    }
  }

  const startAnalysis = async (ids = []) => {
    setAnalysisLoading(true)
    try {
      const res = await fetch(`${API_BASE}/api/ai/analyze/start`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids }),
      })
      if (!res.ok) throw new Error('启动后台分析失败')
      const data = await res.json()
      setAnalysis(data)
    } catch (err) {
      setError(err.message || '启动后台分析失败')
    } finally {
      setAnalysisLoading(false)
    }
  }

  const stopAnalysis = async () => {
    setAnalysisLoading(true)
    try {
      const res = await fetch(`${API_BASE}/api/ai/analyze/stop`, { method: 'POST' })
      if (!res.ok) throw new Error('停止后台分析失败')
      const data = await res.json()
      setAnalysis(data)
    } catch (err) {
      setError(err.message || '停止后台分析失败')
    } finally {
      setAnalysisLoading(false)
    }
  }

  useEffect(() => {
    loadArchives()
    loadAnalysisStatus(true)
  }, [])

  useEffect(() => {
    const handleKeyDown = (e) => {
      if (e.key === 'Escape' && immersiveMode) {
        exitImmersiveMode()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [immersiveMode])

  useEffect(() => {
    if (!analysis?.running) return
    const timer = setInterval(() => {
      loadAnalysisStatus(true)
    }, 5000)
    return () => clearInterval(timer)
  }, [analysis?.running])

  useEffect(() => {
    if (!analysis?.running && analysis?.lastRun && graphMode === 'taxonomy') {
      loadTaxonomy()
    }
  }, [analysis?.running, analysis?.lastRun, graphMode])

  const focusGraph = useMemo(() => buildFocusGraph(selected), [selected])

  useEffect(() => {
    if (view !== 'graph') return
    if (graphMode === 'global') loadGraph()
    if (graphMode === 'taxonomy') loadTaxonomy()
  }, [view, graphMode])

  useEffect(() => {
    if (!selected) {
      setForm({ category: '', tags: '', hierarchy: '' })
      setMetaOpen(false)
      return
    }
    const tags = toTagList(selected.tags)
    let path = ''
    if (Array.isArray(selected.hierarchyPaths) && selected.hierarchyPaths.length > 0) {
      path = selected.hierarchyPaths.join('\n')
    } else if (Array.isArray(selected.hierarchy) && selected.hierarchy.length > 0) {
      path = selected.hierarchy.join('/')
    } else if (selected.hierarchyPath) {
      path = selected.hierarchyPath
    }
    setForm({ category: selected.category || '', tags: tags.join(', '), hierarchy: path })
    setMetaOpen(false)
  }, [selected])

  const stats = useMemo(() => {
    return {
      total: items.length,
      latest: items[0]?.createdAt ? formatDateTime(items[0].createdAt) : '暂无',
    }
  }, [items])

  const saveMeta = async () => {
    if (!selected) return
    setSaving(true)
    try {
      const tags = form.tags
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean)
      const hierarchyPaths = splitHierarchyPaths(form.hierarchy)
      const hierarchy = hierarchyPaths.length > 0 ? hierarchyPaths[0].split('/') : []
      const res = await fetch(`${API_BASE}/api/archives/${selected.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ category: form.category, tags, hierarchy, hierarchyPaths }),
      })
      if (!res.ok) throw new Error('保存失败')
      const updated = await res.json()
      setSelected(updated)
      await loadArchives()
      if (graphMode === 'taxonomy') loadTaxonomy()
    } catch (err) {
      setError(err.message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const runAiTag = async () => {
    if (!selected) return
    setAiLoading(true)
    try {
      const res = await fetch(`${API_BASE}/api/archives/${selected.id}/ai-tag`, {
        method: 'POST',
      })
      if (!res.ok) throw new Error('AI 生成失败')
      const updated = await res.json()
      setSelected(updated)
      await loadArchives()
      if (graphMode === 'taxonomy') loadTaxonomy()
    } catch (err) {
      setError(err.message || 'AI 生成失败')
    } finally {
      setAiLoading(false)
    }
  }

  const deleteArchive = async () => {
    if (!selected) return
    if (!window.confirm('确定要删除该归档吗？')) return
    try {
      const res = await fetch(`${API_BASE}/api/archives/${selected.id}`, {
        method: 'DELETE',
      })
      if (!res.ok) throw new Error('删除失败')
      setSelected(null)
      await loadArchives()
      if (graphMode === 'taxonomy') loadTaxonomy()
    } catch (err) {
      setError(err.message || '删除失败')
    }
  }

  const saveAiConfig = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/ai/config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(aiConfig),
      })
      if (!res.ok) throw new Error('配置保存失败')
      localStorage.setItem('aiBaseUrl', aiConfig.baseUrl)
      localStorage.setItem('aiModel', aiConfig.model)
      localStorage.setItem('aiKey', aiConfig.apiKey)
      setAiOpen(false)
    } catch (err) {
      setError(err.message || '配置保存失败')
    }
  }

  const toggleSelected = (id) => {
    setSelectedIds((prev) => (prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]))
  }

  const selectAll = () => {
    setSelectedIds(items.map((item) => item.id))
  }

  const clearSelection = () => {
    setSelectedIds([])
    setBatchMode(false)
  }

  const enterImmersiveMode = (item) => {
    setSelected(item)
    setImmersiveMode(true)
  }

  const exitImmersiveMode = () => {
    setImmersiveMode(false)
  }

  const runAnalysisSelected = async () => {
    if (selectedIds.length === 0) return
    await startAnalysis(selectedIds)
  }

  const runAnalysisAll = async () => {
    await startAnalysis([])
  }

  const handleNodeClick = (node) => {
    if (node.group === 'archive' && node.refId) {
      const found = items.find((item) => item.id === node.refId)
      if (found) setSelected(found)
      setView('list')
      return
    }
    if (node.group === 'tag') {
      setTag(node.label)
      setView('list')
      loadArchives()
    }
    if (node.group === 'category' || node.group === 'path') {
      setCategory(node.label)
      setView('list')
      loadArchives()
    }
    if (node.group === 'taxonomy' && node.refId) {
      loadTaxonomyNode(node.refId)
    }
  }

  const analysisSummary = analysis?.running
    ? `分析中 · ${analysis.lastLoopProcessed}/${analysis.lastLoopScanned}`
    : analysis?.lastRun
      ? `上次 ${formatDateTime(analysis.lastRun)} · ${analysis.lastLoopProcessed}/${analysis.lastLoopScanned}`
      : '未运行'

  return (
    <div className="app">
      {immersiveMode && selected && (
        <div className="immersive-reader">
          <div className="immersive-header">
            <div className="immersive-info">
              <h1>{selected.title || '未命名页面'}</h1>
              <div className="immersive-meta">
                <span>{selected.siteName || '未知站点'}</span>
                <span>•</span>
                <span>{formatDateTime(selected.createdAt)}</span>
              </div>
            </div>
            <div className="immersive-actions">
              <a href={selected.url} target="_blank" rel="noreferrer" className="ghost small">
                打开原文
              </a>
              <button type="button" className="ghost small" onClick={exitImmersiveMode}>
                退出全屏 (ESC)
              </button>
            </div>
          </div>
          <iframe
            title="immersive-content"
            src={`${API_BASE}/api/archives/${selected.id}/html`}
            className="immersive-content"
          />
        </div>
      )}
      <header className="hero">
        <div className="hero-brand">
          <div className="brand-icon">
            <svg width="32" height="32" viewBox="0 0 32 32" fill="none">
              <rect width="32" height="32" rx="8" fill="url(#gradient)" />
              <path d="M8 12h16M8 16h16M8 20h12" stroke="white" strokeWidth="2" strokeLinecap="round" />
              <circle cx="24" cy="20" r="3" fill="white" />
              <defs>
                <linearGradient id="gradient" x1="0" y1="0" x2="32" y2="32">
                  <stop offset="0%" stopColor="#0969da" />
                  <stop offset="100%" stopColor="#1f6feb" />
                </linearGradient>
              </defs>
            </svg>
          </div>
          <div className="brand-text">
            <h1>WebArchive</h1>
            <p className="brand-slogan">存住价值 · 连成逻辑 · 构建体系</p>
          </div>
        </div>
        <div className="stats">
          <div className="stat-item">
            <div className="stat-icon">📚</div>
            <div className="stat-content">
              <span className="stat-value">{stats.total}</span>
              <span className="stat-label">归档数量</span>
            </div>
          </div>
          <div className="stat-item">
            <div className="stat-icon">⚡</div>
            <div className="stat-content">
              <span className="stat-value">{stats.latest}</span>
              <span className="stat-label">最近更新</span>
            </div>
          </div>
        </div>
      </header>

      <div className="toolbar">
        <div className="tabs">
          <button
            type="button"
            className={view === 'list' ? 'tab active' : 'tab'}
            onClick={() => setView('list')}
          >
            列表视图
          </button>
          <button
            type="button"
            className={view === 'graph' ? 'tab active' : 'tab'}
            onClick={() => setView('graph')}
          >
            知识星球
          </button>
        </div>
        <div className="toolbar-actions">
          <div className={`analysis-pill ${analysis?.running ? 'on' : ''}`}>
            <span className="dot" />
            <div className="analysis-meta">
              <strong>AI 分析</strong>
              <span>{analysisSummary}</span>
            </div>
            <div className="analysis-buttons">
              {selectedIds.length > 0 && batchMode ? (
                <button
                  type="button"
                  className="primary small"
                  onClick={runAnalysisSelected}
                  disabled={analysisLoading || analysis?.running}
                >
                  分析 {selectedIds.length} 篇
                </button>
              ) : (
                <button
                  type="button"
                  className="primary small"
                  onClick={runAnalysisAll}
                  disabled={analysisLoading || analysis?.running}
                >
                  全部分析
                </button>
              )}
              {analysis?.running && (
                <button type="button" className="ghost small" onClick={stopAnalysis}>
                  停止
                </button>
              )}
            </div>
          </div>
          <button type="button" className="ghost" onClick={() => setAiOpen(true)}>
            AI 设置
          </button>
          <button type="button" className="ghost" onClick={loadArchives}>
            刷新
          </button>
        </div>
      </div>

      {view === 'graph' && (
        <section className="panel graph-panel">
          <div className="panel-header">
            <div>
              <h2>知识图谱</h2>
              <p className="panel-sub">点击节点查看子分类或相关文章</p>
            </div>
            <div className="graph-controls">
              <button
                type="button"
                className={graphMode === 'taxonomy' ? 'tab active' : 'tab'}
                onClick={() => setGraphMode('taxonomy')}
              >
                体系
              </button>
              <button
                type="button"
                className={graphMode === 'focus' ? 'tab active' : 'tab'}
                onClick={() => setGraphMode('focus')}
              >
                聚焦
              </button>
              <button
                type="button"
                className={graphMode === 'global' ? 'tab active' : 'tab'}
                onClick={() => setGraphMode('global')}
              >
                全局
              </button>
            </div>
          </div>
          <div className="graph-layout">
            <GraphView
              data={
                graphMode === 'focus'
                  ? focusGraph
                  : graphMode === 'taxonomy'
                    ? buildTaxonomyGraph(taxonomyTree)
                    : graphData
              }
              onNodeClick={handleNodeClick}
            />
            <aside className="graph-side">
              {graphMode === 'taxonomy' && (
                <>
                  <h3>体系节点</h3>
                  {taxonomyLoading && <div className="hint">加载中…</div>}
                  {!taxonomyLoading && taxonomyTree.length === 0 && (
                    <div className="empty-state">
                      <strong>暂无体系</strong>
                      <span>开启后台分析或对文章运行 AI 生成，即可自动构建层级。</span>
                      <button
                        type="button"
                        className="primary"
                        onClick={runAnalysisAll}
                        disabled={analysisLoading || analysis?.running}
                      >
                        {analysis?.running ? '分析中…' : '分析全部文章'}
                      </button>
                    </div>
                  )}
                  {!taxonomyLoading && taxonomyTree.length > 0 && !taxonomyDetail && (
                    <div className="hint">点击节点查看子分类与相关文章</div>
                  )}
                  {taxonomyDetail && (
                    <>
                      <div className="node-title">{taxonomyDetail.node?.label}</div>
                      <div className="node-path">{taxonomyDetail.node?.path}</div>
                      <div className="node-section">
                        <p>子分类</p>
                        <div className="chip-row">
                          {taxonomyDetail.children?.map((child) => (
                            <button
                              key={child.id}
                              type="button"
                              className="chip chip-btn"
                              onClick={() => loadTaxonomyNode(child.id)}
                            >
                              {child.label}
                            </button>
                          ))}
                          {(!taxonomyDetail.children || taxonomyDetail.children.length === 0) && (
                            <span className="hint">无子分类</span>
                          )}
                        </div>
                      </div>
                      <div className="node-section">
                        <p>相关文章</p>
                        <div className="node-articles">
                          {taxonomyDetail.archives?.map((article) => (
                            <button
                              key={article.id}
                              type="button"
                              className="node-article"
                              onClick={() => {
                                setSelected(article)
                                setView('list')
                              }}
                            >
                              <strong>{article.title || '未命名页面'}</strong>
                              <span>{article.siteName || article.url}</span>
                            </button>
                          ))}
                          {(!taxonomyDetail.archives || taxonomyDetail.archives.length === 0) && (
                            <span className="hint">暂无相关文章</span>
                          )}
                        </div>
                      </div>
                    </>
                  )}
                </>
              )}
              {graphMode !== 'taxonomy' && (
                <div className="hint">切换到“体系”查看层级分类</div>
              )}
            </aside>
          </div>
        </section>
      )}

      {view === 'list' && (
        <main className="layout">
          <section className="panel list-panel">
            <div className="panel-header">
              <div>
                <h2>归档列表</h2>
                <p className="panel-sub">支持标题、站点、正文搜索</p>
              </div>
              <div className="list-actions">
                {batchMode ? (
                  <>
                    <div className="selection-actions">
                      <button type="button" className="ghost small" onClick={selectAll} disabled={items.length === 0}>
                        全选
                      </button>
                      <button type="button" className="ghost small" onClick={clearSelection} disabled={selectedIds.length === 0}>
                        取消
                      </button>
                    </div>
                    <span className="selection-count">{selectedIds.length} 项已选</span>
                  </>
                ) : (
                  <>
                    <button type="button" className="ghost small" onClick={() => setBatchMode(true)}>
                      批量操作
                    </button>
                    <button type="button" className="ghost small" onClick={loadArchives}>
                      刷新
                    </button>
                  </>
                )}
              </div>
            </div>
            <div className="filters">
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && loadArchives()}
                placeholder="🔍 搜索标题、站点或正文..."
              />
              <input
                value={category}
                onChange={(e) => setCategory(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && loadArchives()}
                placeholder="分类"
              />
              <input 
                value={tag} 
                onChange={(e) => setTag(e.target.value)} 
                onKeyDown={(e) => e.key === 'Enter' && loadArchives()}
                placeholder="标签" 
              />
              <button type="button" className="primary" onClick={loadArchives}>
                搜索
              </button>
            </div>
            {loading && <div className="hint">加载中…</div>}
            {error && <div className="error">{error}</div>}
            <div className="list">
              {items.map((item) => (
                <div
                  key={item.id}
                  className={`card ${selected?.id === item.id ? 'active' : ''} ${selectedIds.includes(item.id) && batchMode ? 'checked' : ''}`}
                  onClick={() => {
                    if (batchMode) {
                      toggleSelected(item.id)
                    } else {
                      setSelected(item)
                    }
                  }}
                >
                  {batchMode && (
                    <div className="card-select">
                      <input
                        type="checkbox"
                        checked={selectedIds.includes(item.id)}
                        onChange={() => toggleSelected(item.id)}
                        aria-label="选择归档"
                      />
                    </div>
                  )}
                  <div className="card-content">
                    <div className="card-title">{item.title || '未命名页面'}</div>
                    <div className="card-meta">
                      <span>{item.siteName || '未知站点'}</span>
                      <span>•</span>
                      <span>{formatDateTime(item.createdAt)}</span>
                    </div>
                    <div className="card-tags">
                      {item.category && <span className="chip chip-accent">{item.category}</span>}
                      {toTagList(item.tags).slice(0, 3).map((t) => (
                        <span key={t} className="chip">
                          {t}
                        </span>
                      ))}
                    </div>
                  </div>
                  {!batchMode && (
                    <button
                      type="button"
                      className="card-immersive"
                      onClick={(e) => {
                        e.stopPropagation()
                        enterImmersiveMode(item)
                      }}
                      title="全屏阅读"
                    >
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3" />
                      </svg>
                    </button>
                  )}
                </div>
              ))}
              {!loading && items.length === 0 && <div className="hint">暂无归档内容</div>}
            </div>
          </section>

          <section className="panel preview-panel">
            <div className="panel-header">
              <div>
                <h2>内容预览</h2>
                <p className="panel-sub">编辑分类与标签，沉浸式阅读</p>
              </div>
              {selected && (
                <div className="actions">
                  <button
                    type="button"
                    className="primary small"
                    onClick={() => enterImmersiveMode(selected)}
                  >
                    全屏阅读
                  </button>
                  <button type="button" className="ghost small" onClick={() => setMetaOpen((v) => !v)}>
                    {metaOpen ? '收起' : '编辑'}
                  </button>
                  <button type="button" className="ghost small" onClick={runAiTag} disabled={aiLoading}>
                    {aiLoading ? '生成中…' : 'AI 标签'}
                  </button>
                  <button type="button" className="ghost small danger" onClick={deleteArchive}>
                    删除
                  </button>
                  <a href={selected.url} target="_blank" rel="noreferrer" className="ghost small">
                    原文
                  </a>
                </div>
              )}
            </div>
            {selected && metaOpen && (
              <div className="meta-panel">
                <div>
                  <label>分类</label>
                  <input
                    value={form.category}
                    onChange={(e) => setForm((s) => ({ ...s, category: e.target.value }))}
                    placeholder="例如：技术 / 产品 / 设计"
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
                <div>
                  <label>层级路径</label>
                  <textarea
                    rows={2}
                    value={form.hierarchy}
                    onChange={(e) => setForm((s) => ({ ...s, hierarchy: e.target.value }))}
                    placeholder={'例如：计算机/网络/TCP\n计算机/网络/UDP/HTTP3'}
                  />
                </div>
                <button type="button" className="primary" disabled={!selected || saving} onClick={saveMeta}>
                  {saving ? '保存中…' : '保存元信息'}
                </button>
              </div>
            )}
            {selected && (selected.category || toTagList(selected.tags).length > 0) && (
              <div className="chip-row">
                {selected.category && <span className="chip chip-accent">{selected.category}</span>}
                {toTagList(selected.tags).map((t) => (
                  <span key={t} className="chip">
                    {t}
                  </span>
                ))}
              </div>
            )}
            {!selected && <div className="hint">选择左侧内容即可预览</div>}
            {selected && (
              <iframe title="archive-preview" src={`${API_BASE}/api/archives/${selected.id}/html`} />
            )}
          </section>
        </main>
      )}

      {aiOpen && (
        <div className="modal">
          <div className="modal-card">
            <h3>AI 设置</h3>
            <p>用于调用 ChatGPT 格式接口生成标签与层级。</p>
            <label>接口地址</label>
            <input
              value={aiConfig.baseUrl}
              onChange={(e) => setAiConfig((s) => ({ ...s, baseUrl: e.target.value }))}
              placeholder="https://api.openai.com/v1"
            />
            <label>模型名称</label>
            <input
              value={aiConfig.model}
              onChange={(e) => setAiConfig((s) => ({ ...s, model: e.target.value }))}
              placeholder="gpt-4o-mini"
            />
            <label>API Key</label>
            <input
              type="password"
              value={aiConfig.apiKey}
              onChange={(e) => setAiConfig((s) => ({ ...s, apiKey: e.target.value }))}
              placeholder="sk-..."
            />
            <div className="modal-actions">
              <button type="button" className="ghost" onClick={() => setAiOpen(false)}>
                取消
              </button>
              <button type="button" className="primary" onClick={saveAiConfig}>
                保存设置
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
