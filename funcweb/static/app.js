/* ============================================================
   FuncGraph — vanilla JS graph visualiser
   • d3-force layout (async, all nodes always in physics)
   • Sigma WebGL renders only what's in the viewport natively
   • Overlap resolver runs in background after sim cools
   • Isolated nodes pinned in a grid to the right
   ============================================================ */
'use strict';

// ── Palette ──────────────────────────────────────────────────────────────────
// Fixed color per file extension — no red tones.
const EXT_COLORS = {
  '.go':    '#58a6ff',   // blue
  '.py':    '#ffd700',   // yellow
  '.js':    '#f0883e',   // orange
  '.ts':    '#4fc3f7',   // light blue
  '.tsx':   '#79c0ff',   // pale blue
  '.rs':    '#ffa657',   // amber
  '.rb':    '#bc8cff',   // purple
  '.java':  '#56d364',   // green
  '.kt':    '#8957e5',   // violet
  '.cpp':   '#3fb950',   // bright green
  '.cc':    '#3fb950',
  '.cxx':   '#3fb950',
  '.c':     '#85e89d',   // pale green
  '.cs':    '#d2a8ff',   // lavender
  '.scala': '#e3b341',   // gold
  '.lua':   '#b4aaff',   // soft purple
  '.php':   '#a5d6ff',   // sky blue
  '.sh':    '#c8b1e4',   // mauve
  '.bash':  '#c8b1e4',
};
const FALLBACK_PALETTE = [
  '#58a6ff','#3fb950','#d2a8ff','#ffa657','#4fc3f7',
  '#56d364','#bc8cff','#ffd700','#b4aaff','#f0883e',
];

function fileColor(file) {
  const dot = file.lastIndexOf('.');
  const ext = dot >= 0 ? file.slice(dot).toLowerCase() : '';
  if (EXT_COLORS[ext]) return EXT_COLORS[ext];
  // Unknown extension: hash-based fallback (no red).
  let h = 0;
  for (let i = 0; i < ext.length; i++) h = (h * 31 + ext.charCodeAt(i)) >>> 0;
  return FALLBACK_PALETTE[h % FALLBACK_PALETTE.length];
}

// ── Module-level state ───────────────────────────────────────────────────────
let RAW_NODES  = [];
let RAW_EDGES  = [];
let POSITIONS  = {};   // pre-computed layout from server: nodeId → {x, y}
let NODE_BY_ID = new Map();
let OUT_EDGES  = new Map();
let IN_EDGES   = new Map();
let IN_DEG     = new Map();
let OUT_DEG    = new Map();
let ISOLATED   = new Set();

let d3Nodes = [];
let d3Links = [];
let nodeIdx = {};
let sim     = null;
let rafId   = null;

let graph    = null;
let renderer = null;

let hoveredNode    = null;
let selectedNode   = null;
let hoveredSubtree = null;

// ── Node size ─────────────────────────────────────────────────────────────────
function nodeSize(id) {
  const n    = RAW_NODES.length;
  // Scale down as graph grows: 1.0 for ≤100 nodes, ~0.3 for 2000 nodes.
  const scale = Math.max(0.25, 1 / Math.sqrt(n / 100));
  const base  = (5 + Math.min(Math.log1p((IN_DEG.get(id)||0)+(OUT_DEG.get(id)||0)) * 2, 12));
  return Math.max(2, base * scale);
}

// ── Physics ───────────────────────────────────────────────────────────────────
function buildPhysics() {
  const oldPos = {};
  for (const dn of d3Nodes) oldPos[dn.id] = { x: dn.x, y: dn.y, vx: dn.vx||0, vy: dn.vy||0 };

  d3Nodes = [];
  nodeIdx = {};
  for (const n of RAW_NODES) {
    const o   = oldPos[n.id];
    const pre = POSITIONS[n.id];       // server-computed position
    let x, y, vx = 0, vy = 0;
    if (o) {
      x = o.x; y = o.y; vx = o.vx; vy = o.vy;
    } else if (pre) {
      x = pre.x; y = pre.y;           // use backend layout directly
    } else {
      x = (Math.random()-.5)*700; y = (Math.random()-.5)*700;
    }
    d3Nodes.push({ id: n.id, x, y, vx, vy });
    nodeIdx[n.id] = d3Nodes.length - 1;
  }

  d3Links = [];
  for (const e of RAW_EDGES) {
    const si = nodeIdx[e.from], ti = nodeIdx[e.to];
    if (si !== undefined && ti !== undefined && e.from !== e.to)
      d3Links.push({ source: si, target: ti });
  }

  const n      = d3Nodes.length;
  const charge = -Math.max(400, n * 4);
  const theta  = n > 800 ? 0.95 : n > 300 ? 0.9 : 0.8;

  if (sim) sim.stop();
  sim = d3.forceSimulation(d3Nodes)
    .force('link',    d3.forceLink(d3Links).id((_,i)=>i).distance(80).strength(0.4))
    .force('charge',  d3.forceManyBody().strength(charge).theta(theta))
    .force('collide', d3.forceCollide(22).strength(1.0).iterations(4))
    .force('center',  d3.forceCenter(0,0).strength(0.02))
    .alphaDecay(0.008)
    .velocityDecay(0.35)
    .stop();

  // Pin isolated nodes in a grid to the right of the main cluster.
  const isolated = d3Nodes.filter(dn => ISOLATED.has(dn.id));
  if (isolated.length) {
    const nConn  = n - isolated.length;
    const radius = Math.max(300, Math.sqrt(nConn+1) * 80);
    const cols   = Math.max(1, Math.ceil(Math.sqrt(isolated.length)));
    const step   = 28;
    const oy     = -(Math.ceil(isolated.length/cols)*step)/2;
    isolated.forEach((dn, i) => {
      dn.fx = radius + 120 + (i%cols)*step;
      dn.fy = oy + Math.floor(i/cols)*step;
      dn.x  = dn.fx; dn.y = dn.fy;
    });
  }
}

// Sync physics positions → graphology so Sigma re-renders.
function syncToGraph() {
  if (!graph) return;
  for (const dn of d3Nodes) {
    if (graph.hasNode(dn.id)) {
      graph.setNodeAttribute(dn.id, 'x', dn.x);
      graph.setNodeAttribute(dn.id, 'y', dn.y);
    }
  }
}

function startSim(alpha = 0.8) {
  if (!sim) return;
  cancelAnimationFrame(rafId);
  sim.alpha(alpha);
  (function tick() {
    sim.tick();
    syncToGraph();
    if (renderer) renderer.refresh();
    if (sim.alpha() > sim.alphaMin()) {
      rafId = requestAnimationFrame(tick);
    }
  })();
}


// ── Add all nodes/edges to graphology ────────────────────────────────────────
function populateGraph() {
  if (!graph) return;
  graph.clear();
  for (const n of RAW_NODES) {
    const dn = d3Nodes[nodeIdx[n.id]];
    const iso = ISOLATED.has(n.id);
    graph.addNode(n.id, {
      label:      n.name,
      x:          dn ? dn.x : 0,
      y:          dn ? dn.y : 0,
      size:       iso ? 4 : nodeSize(n.id),
      color:      iso ? '#3a4455' : fileColor(n.file),
      _file:      n.file,
      _lang:      n.language,
      _startLine: n.start_line,
      _endLine:   n.end_line,
      _isolated:  iso,
    });
  }
  for (const e of RAW_EDGES) {
    if (!graph.hasNode(e.from) || !graph.hasNode(e.to) || e.from===e.to) continue;
    const eid = e.id || `${e.from}>${e.to}`;
    if (!graph.hasEdge(eid))
      graph.addEdgeWithKey(eid, e.from, e.to, {
        color: 'rgba(100,120,145,0.5)', size: 1.2,
      });
  }
}

// Fit camera to show all nodes. Called after first render completes.
function pointToSegmentDist(px, py, ax, ay, bx, by) {
  const dx = bx-ax, dy = by-ay;
  const lenSq = dx*dx + dy*dy;
  if (lenSq === 0) return Math.hypot(px-ax, py-ay);
  const t = Math.max(0, Math.min(1, ((px-ax)*dx + (py-ay)*dy) / lenSq));
  return Math.hypot(px - (ax+t*dx), py - (ay+t*dy));
}

function fitCamera() {
  if (!graph || !renderer || graph.order === 0) return;
  // animatedReset fits all nodes — Sigma computes bounds internally.
  renderer.getCamera().animatedReset();
}

function updateStatusBar(stNodes, stEdges, nodeCountEl) {
  if (!graph) return;
  let iso = 0;
  graph.forEachNode((_,a)=>{ if(a._isolated) iso++; });
  if (stNodes)    stNodes.textContent    = `nodes: ${graph.order-iso}${iso?' + '+iso+' iso':''}`;
  if (stEdges)    stEdges.textContent    = `edges: ${graph.size}`;
  if (nodeCountEl) nodeCountEl.textContent = `${RAW_NODES.length} total`;
}

// ── Error surfacing ───────────────────────────────────────────────────────────
window.addEventListener('error', ev => {
  const el = document.getElementById('loading-text');
  if (el) el.textContent = 'JS error: ' + ev.message + ' (' + ev.lineno + ')';
});
window.addEventListener('unhandledrejection', ev => {
  const el = document.getElementById('loading-text');
  if (el) el.textContent = 'Promise error: ' + (ev.reason||ev);
});

// ── Bootstrap ─────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', async () => {
  const loadingEl    = document.getElementById('loading');
  const loadingText  = document.getElementById('loading-text');
  const sigmaEl      = document.getElementById('sigma-canvas');
  const codePanel    = document.getElementById('code-panel');
  const panelFnName  = document.getElementById('panel-fn-name');
  const panelFilePath= document.getElementById('panel-file-path');
  const panelMeta    = document.getElementById('panel-meta');
  const codeOutput   = document.getElementById('code-output');
  const codeScroll   = document.getElementById('code-scroll');
  const tooltip      = document.getElementById('tooltip');
  const ttName       = document.getElementById('tt-name');
  const ttFile       = document.getElementById('tt-file');
  const ttStats      = document.getElementById('tt-stats');
  const stNodes      = document.getElementById('st-nodes');
  const stEdges      = document.getElementById('st-edges');
  const stZoom       = document.getElementById('st-zoom');
  const stHover      = document.getElementById('st-hover');
  const nodeCountEl  = document.getElementById('node-count');
  const searchInput  = document.getElementById('search');
  const searchResults= document.getElementById('search-results');

  try {

  // 1. Fetch ────────────────────────────────────────────────────────────
  loadingText.textContent = 'Loading graph…';
  let raw;
  try {
    const res = await fetch('/api/graph');
    if (!res.ok) throw new Error(await res.text());
    raw = await res.json();
  } catch(e) { loadingText.textContent='Error: '+e.message; return; }

  RAW_NODES  = raw.nodes     || [];
  RAW_EDGES  = raw.edges     || [];
  POSITIONS  = raw.positions || {};
  NODE_BY_ID = new Map(RAW_NODES.map(n=>[n.id,n]));

  const connectedInRaw = new Set();
  for (const e of RAW_EDGES) { connectedInRaw.add(e.from); connectedInRaw.add(e.to); }
  ISOLATED = new Set(RAW_NODES.filter(n=>!connectedInRaw.has(n.id)).map(n=>n.id));

  for (const e of RAW_EDGES) {
    OUT_DEG.set(e.from,(OUT_DEG.get(e.from)||0)+1);
    IN_DEG.set(e.to,  (IN_DEG.get(e.to)  ||0)+1);
    if (!OUT_EDGES.has(e.from)) OUT_EDGES.set(e.from,[]);
    OUT_EDGES.get(e.from).push(e);
    if (!IN_EDGES.has(e.to)) IN_EDGES.set(e.to,[]);
    IN_EDGES.get(e.to).push(e);
  }

  // 2. Graphology + Sigma ───────────────────────────────────────────────
  loadingText.textContent = 'Rendering…';
  graph = new graphology.Graph({ type:'directed', multi:false });

  renderer = new Sigma(graph, sigmaEl, {
    renderEdgeLabels:            false,
    defaultEdgeType:             'arrow',
    labelFont:                   'system-ui, sans-serif',
    labelSize:                   13,
    labelWeight:                 '500',
    labelColor:                  { color:'#c9d1d9' },
    labelRenderedSizeThreshold:  Infinity,  // labels hidden by default
    minCameraRatio:              0.001,  // ~1000× zoom
    maxCameraRatio:              15,
    zIndexingRatio:              0.5,    // enables edge/node z-ordering

    nodeReducer: (node, data) => {
      const r = { ...data };

      // Sigma renders size in graph-space — nodes grow as you zoom in.
      // Multiply by ratio to fully cancel that growth: nodes stay the same
      // screen-pixel size regardless of zoom level.
      const ratio = renderer ? renderer.getCamera().ratio : 1;
      // zoom in (ratio < 1): shrink nodes so they don't become huge blobs.
      // zoom out (ratio > 1): keep original size — don't scale up.
      const sizeScale = ratio < 1 ? ratio * ratio * ratio : 1;
      r.size = Math.max(0.3, data.size * sizeScale);

      // Show label only for hovered node and its subtree.
      if (!hoveredSubtree || !hoveredSubtree.nodes.has(node)) {
        r.label = '';
      }

      if (hoveredNode && hoveredSubtree && !hoveredSubtree.nodes.has(node)) {
        r.color='#1e2a35'; return r;
      }
      if (node===selectedNode) {
        r.highlighted = true;
        r.color = '#ffffff';
      }
      if (node===hoveredNode && node!==selectedNode) { r.size*=1.6; r.color='#fff'; }
      return r;
    },

    edgeReducer: (edge, data) => {
      const r     = { ...data };
      const ratio = renderer ? renderer.getCamera().ratio : 1;
      const scale = Math.min(1, ratio * ratio * ratio);

      if (hoveredNode && hoveredSubtree) {
        if (hoveredSubtree.edges.has(edge)) {
          // Callee edges (outgoing from selected): blue
          // Caller edges (incoming to selected): orange
          const isCaller = graph.target(edge) === hoveredNode;
          r.color  = isCaller ? '#ffa657' : '#58a6ff';
          r.size   = Math.max(0.2, 1.2 * scale);
          r.zIndex = 9999;
        } else {
          r.hidden = true;
        }
      } else {
        r.color = '#2a3a4a';
        r.size  = Math.max(0.1, data.size * scale);
      }
      return r;
    },
  });

  renderer.getCamera().on('updated', s => {
    stZoom.textContent = `zoom: ${(1/s.ratio).toFixed(2)}`;
  });

  // 3. Physics + initial layout ─────────────────────────────────────────
  loadingText.textContent = 'Computing layout…';
  buildPhysics();

  const hasServerLayout = Object.keys(POSITIONS).length > 0;
  const pre = hasServerLayout ? 0
    : RAW_NODES.length < 100 ? 300 : RAW_NODES.length < 300 ? 150
    : RAW_NODES.length < 800 ? 80 : 40;
  for (let i=0;i<pre;i++) sim.tick();

  // All nodes go into graphology with their computed positions.
  populateGraph();
  updateStatusBar(stNodes, stEdges, nodeCountEl);

  loadingEl.classList.add('hidden');
  setTimeout(()=>{ loadingEl.style.display='none'; },400);

  // Wait one frame so Sigma has rendered at least once, then fit.
  requestAnimationFrame(() => fitCamera());

  // 4. Hover ────────────────────────────────────────────────────────────
  function computeSubtree(id) {
    const nodes = new Set([id]), edges = new Set();
    // Outgoing: depth 1 only (direct callees).
    graph.outEdges(id).forEach(eid => {
      edges.add(eid); nodes.add(graph.target(eid));
    });
    // Incoming: depth 1 only (direct callers).
    graph.inEdges(id).forEach(eid => {
      edges.add(eid); nodes.add(graph.source(eid));
    });
    return {nodes, edges};
  }

  renderer.on('enterNode',({node,event})=>{
    const a=graph.getNodeAttributes(node);
    ttName.textContent =a.label;
    ttFile.textContent =a._file;
    ttStats.textContent=a._isolated
      ?'no connections (isolated)'
      :`callers: ${(IN_EDGES.get(node)||[]).length}  callees: ${(OUT_EDGES.get(node)||[]).length}`;
    tooltip.classList.add('visible'); moveTooltip(event); stHover.textContent=a.label;
  });

  renderer.on('leaveNode',()=>{
    tooltip.classList.remove('visible'); stHover.textContent='';
  });

  renderer.on('moveBody',({event})=>{
    if(tooltip.classList.contains('visible')) moveTooltip(event);
  });

  function moveTooltip(e){
    tooltip.style.left=Math.min(e.x+14,window.innerWidth -290)+'px';
    tooltip.style.top =Math.min(e.y+14,window.innerHeight-90 )+'px';
  }

  // 5. Click ────────────────────────────────────────────────────────────
  renderer.on('clickNode',({node})=>{
    selectedNode=node;
    hoveredNode=node;
    hoveredSubtree=computeSubtree(node);
    renderer.refresh();
    openCodePanel(node);
  });

  renderer.on('clickStage',()=>{
    selectedNode=null; hoveredNode=null; hoveredSubtree=null;
    renderer.refresh();
  });

  // 6. Saved nodes ──────────────────────────────────────────────────────
  const savedView    = document.getElementById('saved-view');
  const svTabBar     = document.getElementById('sv-tab-bar');
  const svList       = document.getElementById('sv-list');
  const panelSaveBtn = document.getElementById('panel-save-node');
  let savedMode = false;
  let activeTabId = 'entry-points';

  // In-memory cache; synced to backend via persistSaved().
  let _savedCache = null;

  function loadSaved() {
    if (_savedCache) return _savedCache;
    return { tabs: [{ id:'entry-points', name:'Entry Points', permanent:true, nodes:[] }] };
  }

  async function fetchSaved() {
    try {
      const res = await fetch('/api/saved');
      if (!res.ok) throw new Error(res.status);
      const data = await res.json();
      // Ensure Entry Points tab always exists as first permanent tab.
      if (!data.tabs) data.tabs = [];
      if (!data.tabs.find(t => t.id === 'entry-points')) {
        data.tabs.unshift({ id:'entry-points', name:'Entry Points', permanent:true, nodes:[] });
      }
      _savedCache = data;
    } catch(_) {
      _savedCache = { tabs: [{ id:'entry-points', name:'Entry Points', permanent:true, nodes:[] }] };
    }
  }

  function persistSaved(data) {
    _savedCache = data;
    fetch('/api/saved', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    }).catch(e => console.warn('save failed:', e));
  }

  // Auto-populate Entry Points with nodes that have no callers
  function refreshEntryPoints(data) {
    const ep = data.tabs.find(t => t.id === 'entry-points');
    if (!ep) return;
    ep.nodes = RAW_NODES
      .filter(n => !(IN_EDGES.get(n.id)||[]).length)
      .map(n => n.id)
      .slice(0, 200); // cap
  }

  function renderSavedUI() {
    const data = loadSaved();
    // Tab bar
    svTabBar.innerHTML = '';
    for (const tab of data.tabs) {
      const btn = document.createElement('button');
      btn.className = 'sv-tab' + (tab.id === activeTabId ? ' active' : '');
      btn.dataset.tabId = tab.id;
      btn.textContent = tab.name;
      if (!tab.permanent) {
        const del = document.createElement('button');
        del.className = 'sv-tab-close';
        del.textContent = '✕';
        del.title = 'Delete tab';
        del.onclick = e => { e.stopPropagation(); deleteTab(tab.id); };
        btn.appendChild(del);
      }
      btn.addEventListener('click', () => { activeTabId = tab.id; renderSavedUI(); });
      svTabBar.appendChild(btn);
    }
    const addBtn = document.createElement('button');
    addBtn.id = 'sv-add-tab'; addBtn.textContent = '+'; addBtn.title = 'New tab';
    addBtn.onclick = addTab;
    svTabBar.appendChild(addBtn);

    // Node list
    const tab = data.tabs.find(t => t.id === activeTabId);
    svList.innerHTML = '';
    if (!tab || !tab.nodes.length) {
      svList.innerHTML = '<div class="sv-empty">No saved functions</div>';
      return;
    }
    for (const id of tab.nodes) {
      const n = NODE_BY_ID.get(id);
      if (!n) continue;
      const item = document.createElement('div');
      item.className = 'sv-item' + (id === selectedNode ? ' active' : '');
      item.innerHTML = `
        <div style="flex:1;min-width:0">
          <div class="sv-item-name">${esc(n.name)}</div>
          <div class="sv-item-file">${esc(n.file)}</div>
        </div>
        ${tab.permanent ? '' : `<button class="sv-item-del" title="Remove">✕</button>`}
      `;
      item.querySelector('.sv-item-name')?.parentElement?.addEventListener('click', () => {
        selectSavedNode(id);
      });
      item.addEventListener('click', () => selectSavedNode(id));
      if (!tab.permanent) {
        item.querySelector('.sv-item-del')?.addEventListener('click', e => {
          e.stopPropagation(); removeNodeFromTab(id);
        });
      }
      svList.appendChild(item);
    }
  }

  function selectSavedNode(id) {
    if (!graph.hasNode(id)) return;
    // Only set selectedNode — don't set hoveredNode/hoveredSubtree so the
    // rest of the graph stays fully visible (no dimming).
    selectedNode = id;
    renderer.refresh();
    renderSavedUI();

    try {
      // getNodeDisplayData returns coordinates in Sigma's internal normalized space —
      // the same space camera.x/y live in. No manual normalization needed.
      const display = renderer.getNodeDisplayData(id);
      if (!display) return;
      const cam = renderer.getCamera();
      cam.animate(
        { x: display.x, y: display.y, ratio: cam.getState().ratio },
        { duration: 500 }
      );
    } catch(e) { console.error('navigate error', e); }
  }

  function addTab() {
    const name = prompt('Tab name:');
    if (!name?.trim()) return;
    const data = loadSaved();
    data.tabs.push({ id: 'tab-' + Date.now(), name: name.trim(), permanent: false, nodes: [] });
    persistSaved(data);
    renderSavedUI();
  }

  function deleteTab(tabId) {
    const data = loadSaved();
    data.tabs = data.tabs.filter(t => t.id !== tabId);
    if (activeTabId === tabId) activeTabId = 'entry-points';
    persistSaved(data);
    renderSavedUI();
  }

  function removeNodeFromTab(nodeId) {
    const data = loadSaved();
    const tab = data.tabs.find(t => t.id === activeTabId);
    if (tab) tab.nodes = tab.nodes.filter(id => id !== nodeId);
    persistSaved(data);
    renderSavedUI();
  }

  // Save popup
  const savePopup = document.getElementById('save-popup');

  function openSavePopup(anchorEl) {
    if (!selectedNode) return;
    const data = loadSaved();
    const userTabs = data.tabs.filter(t => !t.permanent);

    savePopup.innerHTML = `<div class="sp-title">Save to tab</div>`;

    if (userTabs.length === 0) {
      const empty = document.createElement('div');
      empty.style.cssText = 'padding:8px 14px;font-size:12px;color:var(--text-dim)';
      empty.textContent = 'No tabs yet';
      savePopup.appendChild(empty);
    }

    for (const tab of userTabs) {
      const already = tab.nodes.includes(selectedNode);
      const btn = document.createElement('button');
      btn.className = 'sp-tab-btn' + (already ? ' saved' : '');
      btn.textContent = (already ? '✓ ' : '') + tab.name;
      btn.onclick = () => {
        if (!already) {
          tab.nodes.push(selectedNode);
          persistSaved(data);
        }
        closeSavePopup();
        if (savedMode) renderSavedUI();
      };
      savePopup.appendChild(btn);
    }

    const newBtn = document.createElement('button');
    newBtn.className = 'sp-new-tab';
    newBtn.textContent = '＋ New tab…';
    newBtn.onclick = () => {
      closeSavePopup();
      const name = prompt('Tab name:');
      if (!name?.trim()) return;
      const d = loadSaved();
      const newTab = { id: 'tab-' + Date.now(), name: name.trim(), permanent: false, nodes: [selectedNode] };
      d.tabs.push(newTab);
      persistSaved(d);
      activeTabId = newTab.id;
      if (savedMode) renderSavedUI();
    };
    savePopup.appendChild(newBtn);

    // Position below the anchor button
    const rect = anchorEl.getBoundingClientRect();
    savePopup.style.display = 'block';
    savePopup.style.top  = (rect.bottom + 6) + 'px';
    savePopup.style.right = (window.innerWidth - rect.right) + 'px';
    savePopup.style.left = 'auto';
  }

  function closeSavePopup() {
    savePopup.style.display = 'none';
  }

  document.addEventListener('click', e => {
    if (!savePopup.contains(e.target) && e.target !== panelSaveBtn) closeSavePopup();
  });

  panelSaveBtn.addEventListener('click', e => {
    e.stopPropagation();
    if (savePopup.style.display === 'none') openSavePopup(panelSaveBtn);
    else closeSavePopup();
  });

  function setSavedMode(on) {
    savedMode = on;
    savedView.style.display = on ? 'flex' : 'none';
    codeScroll.style.display = on ? 'none' : '';
    document.getElementById('panel-meta').style.display = on ? 'none' : '';
    document.getElementById('panel-fn-name').textContent = on ? 'Saved' : '';
    document.getElementById('panel-file-path').textContent = '';
    panelSaveBtn.style.display = on ? 'none' : (selectedNode ? 'block' : 'none');
    document.getElementById('btn-saved').classList.toggle('active', on);
    if (on) {
      codePanel.classList.add('open');
      const data = loadSaved();
      refreshEntryPoints(data);
      persistSaved(data);
      renderSavedUI();
    }
  }

  document.getElementById('btn-saved').addEventListener('click', async () => {
    if (!savedMode) await fetchSaved();
    setSavedMode(!savedMode);
  });

  // 7. Code panel ───────────────────────────────────────────────────────
  document.getElementById('panel-close').addEventListener('click',()=>{
    codePanel.classList.remove('open');
    selectedNode=null; hoveredNode=null; hoveredSubtree=null;
    renderer.refresh();
  });

  async function openCodePanel(nodeId){
    const a=graph.getNodeAttributes(nodeId);
    // Always show code view, hide saved view.
    savedMode = false;
    savedView.style.display   = 'none';
    codeScroll.style.display  = '';
    document.getElementById('panel-meta').style.display = '';
    document.getElementById('btn-saved').classList.remove('active');
    panelSaveBtn.style.display = 'inline-block';
    codePanel.classList.add('open');
    panelFnName.textContent   =a.label;
    panelFilePath.textContent =a._file;
    panelMeta.innerHTML=`
      <span class="meta-chip">lang: <b>${a._lang||'?'}</b></span>
      <span class="meta-chip">lines: <b>${a._startLine}–${a._endLine}</b></span>
      <span class="meta-chip">callers: <b>${(IN_EDGES.get(nodeId)||[]).length}</b></span>
      <span class="meta-chip">callees: <b>${(OUT_EDGES.get(nodeId)||[]).length}</b></span>
    `;
    codeOutput.innerHTML='<div class="code-loading">Loading…</div>';
    try{
      const res=await fetch(`/api/file?path=${encodeURIComponent(a._file)}`);
      if(!res.ok){codeOutput.innerHTML='<div class="code-loading">Could not load.</div>';return;}
      const data=await res.json();
      renderCode(data.content,data.language,a._startLine,a._endLine);
      // Fire-and-forget: classify entry point, add to saved tab if positive.
      fetch(`/api/analyze/${encodeURIComponent(nodeId)}`)
        .then(r=>r.ok?r.json():null)
        .then(res=>{ if(res?.is_entry_point && savedMode) renderSavedUI(); })
        .catch(()=>{});
    }catch(e){codeOutput.innerHTML=`<div class="code-loading">Error: ${e.message}</div>`;}
  }

  function renderCode(content,language,fnStart,fnEnd){
    let hl;
    try{hl=hljs.highlight(content,{language,ignoreIllegals:true}).value;}
    catch(_){hl=hljs.highlightAuto(content).value;}
    const lines=splitLines(hl);
    codeOutput.innerHTML='<div class="code-table">'+lines.map((lh,i)=>{
      const n=i+1,fn=n>=fnStart&&n<=fnEnd;
      let cls='code-row'+(fn?' fn-line':'');
      if(n===fnStart)cls+=' fn-line-start';
      if(n===fnEnd)  cls+=' fn-line-end';
      return `<div class="${cls}"><span class="line-num">${n}</span><span class="line-code">${lh||' '}</span></div>`;
    }).join('')+'</div>';
    const t=codeOutput.querySelector('.fn-line-start');
    if(t)requestAnimationFrame(()=>{codeScroll.scrollTop=Math.max(0,t.offsetTop-codeScroll.clientHeight*.3);});
  }

  function splitLines(html){
    const out=[]; let cur='',tags=[],i=0;
    while(i<html.length){
      if(html[i]==='\n'){out.push(cur);cur=tags.map(t=>`<span class="${t}">`).join('');i++;continue;}
      if(html[i]==='<'){
        const end=html.indexOf('>',i);if(end<0){cur+=html[i++];continue;}
        const tag=html.slice(i,end+1);
        if(tag.startsWith('<span')){const m=tag.match(/class="([^"]*)"/);if(m)tags.push(m[1]);}
        else if(tag==='</span>')tags.pop();
        cur+=tag;i=end+1;continue;
      }
      cur+=html[i++];
    }
    if(cur)out.push(cur);
    if(out.length&&tags.length)out[out.length-1]+='</span>'.repeat(tags.length);
    return out;
  }

  // 8. Search ───────────────────────────────────────────────────────────
  searchInput.addEventListener('input',()=>{
    const q=searchInput.value.trim().toLowerCase();
    if(!q){searchResults.classList.remove('open');return;}
    const matches=RAW_NODES.filter(n=>n.name.toLowerCase().includes(q)||n.file.toLowerCase().includes(q)).slice(0,20);
    if(!matches.length){searchResults.classList.remove('open');return;}
    searchResults.innerHTML=matches.map(m=>
      `<div class="sr-item" data-id="${m.id}"><div class="sr-name">${esc(m.name)}</div><div class="sr-file">${esc(m.file)}</div></div>`
    ).join('');
    searchResults.classList.add('open');
  });

  searchResults.addEventListener('click',e=>{
    const item=e.target.closest('.sr-item');
    if(!item)return;
    searchResults.classList.remove('open'); searchInput.value='';
    const id=item.dataset.id;
    if(!graph.hasNode(id)) return;
    selectedNode=id; hoveredNode=id; hoveredSubtree=computeSubtree(id);
    renderer.refresh();
    openCodePanel(id);
  });

  document.addEventListener('click',e=>{
    if(!searchResults.contains(e.target)&&e.target!==searchInput)
      searchResults.classList.remove('open');
  });

  // 8b. Node dragging ───────────────────────────────────────────────────
  // We use a transparent overlay div that sits on top of the sigma canvas
  // during drag — this fully intercepts mouse events so Sigma never sees
  // the mousemove and therefore cannot pan the camera.

  let dragNode    = null;
  let dragLastPos = null;

  // Capture phase — runs BEFORE Sigma's own mousedown handler.
  // If the click is on the selected node, block Sigma from starting a camera pan.
  sigmaEl.addEventListener('mousedown', e => {
    if (!selectedNode) return;
    try {
      const screen = renderer.graphToViewport({
        x: graph.getNodeAttribute(selectedNode, 'x'),
        y: graph.getNodeAttribute(selectedNode, 'y'),
      });
      const dist = Math.hypot(e.offsetX - screen.x, e.offsetY - screen.y);
      const size = graph.getNodeAttribute(selectedNode, 'size') || 8;
      if (dist > size * 3 + 10) return;
    } catch(_) { return; }

    e.stopImmediatePropagation(); // prevents Sigma from registering camera drag
    dragNode    = selectedNode;
    dragLastPos = { x: e.offsetX, y: e.offsetY };
    document.body.style.cursor = 'grabbing';
  }, true /* capture */);

  document.addEventListener('mousemove', e => {
    if (!dragNode || !dragLastPos) return;
    const rect = sigmaEl.getBoundingClientRect();
    const cur  = { x: e.clientX - rect.left, y: e.clientY - rect.top };
    try {
      const p1 = renderer.viewportToGraph(dragLastPos);
      const p2 = renderer.viewportToGraph(cur);
      const nx = graph.getNodeAttribute(dragNode, 'x') + (p2.x - p1.x);
      const ny = graph.getNodeAttribute(dragNode, 'y') + (p2.y - p1.y);
      graph.setNodeAttribute(dragNode, 'x', nx);
      graph.setNodeAttribute(dragNode, 'y', ny);
      const idx = nodeIdx[dragNode];
      if (idx !== undefined) {
        d3Nodes[idx].x = d3Nodes[idx].fx = nx;
        d3Nodes[idx].y = d3Nodes[idx].fy = ny;
      }
      renderer.refresh();
    } catch(_) {}
    dragLastPos = cur;
  });

  document.addEventListener('mouseup', () => {
    if (!dragNode) return;
    dragNode = null; dragLastPos = null;
    document.body.style.cursor = '';
  });

  // 9. Panel resize ─────────────────────────────────────────────────────
  const rh=document.getElementById('panel-resize');
  let resizing=false,rx=0,rw=0;
  rh.addEventListener('mousedown',e=>{
    e.preventDefault();resizing=true;rx=e.clientX;rw=codePanel.offsetWidth;
    rh.classList.add('dragging');document.body.style.cssText='cursor:ew-resize;user-select:none';
  });
  document.addEventListener('mousemove',e=>{
    if(!resizing)return;
    const min=parseInt(getComputedStyle(codePanel).minWidth)||260;
    codePanel.style.width=Math.min(Math.round(window.innerWidth*.9),Math.max(min,rw+rx-e.clientX))+'px';
  });
  document.addEventListener('mouseup',()=>{
    if(!resizing)return;resizing=false;rh.classList.remove('dragging');document.body.style.cssText='';
  });

  function esc(s){return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}

  }catch(e){
    loadingText.textContent='Init error: '+e.message;
    console.error(e);
  }
});
