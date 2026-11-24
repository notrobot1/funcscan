/* ============================================================
   FuncGraph — vanilla JS graph visualiser
   Depends on: graphology, sigma, d3-force, highlight.js
               (all served from /vendor/ — no CDN required)
   ============================================================ */

'use strict';

// ── Palette ─────────────────────────────────────────────────────────────────
// Assign a stable colour to each unique file/language.
const PALETTE = [
  '#58a6ff','#3fb950','#f78166','#d2a8ff','#ffa657',
  '#79c0ff','#56d364','#ff7b72','#bc8cff','#f0883e',
  '#39d353','#ff6e6e','#b4aaff','#ffd700','#4fc3f7',
];

function fileColor(file) {
  let h = 0;
  for (let i = 0; i < file.length; i++) h = (h * 31 + file.charCodeAt(i)) >>> 0;
  return PALETTE[h % PALETTE.length];
}

// ── Level-of-detail clustering ───────────────────────────────────────────────
// When camera.ratio > LOD_THRESHOLD the viewport is very zoomed-out.
// We merge spatially-close nodes into a single representative so the renderer
// does not need to draw thousands of overlapping dots.
const LOD_THRESHOLD = 0.6;    // camera ratio above which we start merging
const LOD_CELL_PX   = 48;     // screen-pixels per merge cell (larger = more aggressive)

class LODManager {
  constructor() {
    this.hiddenNodes  = new Set();  // nodeId → hidden
    this.clusterSize  = new Map();  // representativeId → count
  }

  // Recompute which nodes are hidden based on current camera state.
  // graphNodes: array of { id, x, y } in graph-space.
  recompute(graphNodes, cameraRatio) {
    this.hiddenNodes.clear();
    this.clusterSize.clear();

    if (cameraRatio < LOD_THRESHOLD) return;

    // The graph-space cell size that maps to LOD_CELL_PX screen pixels.
    const cellSizeGraph = LOD_CELL_PX * cameraRatio;

    // Bucket nodes into cells; keep the highest-degree node per cell.
    const cells = new Map();   // cellKey → { repId, repDegree, count }
    for (const { id, x, y, degree } of graphNodes) {
      const cx = Math.floor(x / cellSizeGraph);
      const cy = Math.floor(y / cellSizeGraph);
      const key = `${cx},${cy}`;
      const existing = cells.get(key);
      if (!existing || degree > existing.repDegree) {
        if (existing) this.hiddenNodes.add(existing.repId); // demote old rep
        cells.set(key, { repId: id, repDegree: degree, count: (existing ? existing.count : 0) + 1 });
        if (existing) this.hiddenNodes.delete(id); // new rep is visible
      } else {
        this.hiddenNodes.add(id);
        existing.count++;
      }
    }
    for (const { repId, count } of cells.values()) {
      if (count > 1) this.clusterSize.set(repId, count);
    }
  }
}

// ── Hover / selection state ──────────────────────────────────────────────────
let hoveredNode     = null;   // node id under mouse
let selectedNode    = null;   // node id clicked
let hoveredSubtree  = null;   // { nodes: Set, edges: Set }

function computeSubtree(graph, nodeId) {
  const nodes = new Set();
  const edges = new Set();
  const queue = [nodeId];
  const visited = new Set([nodeId]);

  while (queue.length) {
    const cur = queue.shift();
    nodes.add(cur);
    // Outgoing (callees)
    graph.outEdges(cur).forEach(eid => {
      const tgt = graph.target(eid);
      edges.add(eid);
      if (!visited.has(tgt)) { visited.add(tgt); queue.push(tgt); }
    });
    // Incoming (callers)
    graph.inEdges(cur).forEach(eid => {
      const src = graph.source(eid);
      edges.add(eid);
      if (!visited.has(src)) { visited.add(src); queue.push(src); }
    });
  }
  return { nodes, edges };
}

// ── Main bootstrap ───────────────────────────────────────────────────────────
// Surface any uncaught errors during init into the loading overlay.
window.addEventListener('error', ev => {
  const el = document.getElementById('loading-text');
  if (el) el.textContent = 'JS error: ' + (ev.message || ev) + ' (' + (ev.filename||'') + ':' + (ev.lineno||'') + ')';
  console.error('uncaught error', ev);
});
window.addEventListener('unhandledrejection', ev => {
  const el = document.getElementById('loading-text');
  if (el) el.textContent = 'Promise error: ' + (ev.reason || ev);
  console.error('unhandled rejection', ev.reason);
});

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

  // Wrap entire init so any crash surfaces in the loading overlay.
  try {

  // ── 1. Fetch graph data ────────────────────────────────────────────────
  loadingText.textContent = 'Loading graph…';
  let graphData;
  try {
    const res = await fetch('/api/graph');
    if (!res.ok) throw new Error(await res.text());
    graphData = await res.json();
  } catch (e) {
    loadingText.textContent = 'Error loading graph: ' + e.message;
    return;
  }

  const { nodes: rawNodes, edges: rawEdges } = graphData;

  // ── 2. Build Graphology instance ───────────────────────────────────────
  loadingText.textContent = 'Building graph…';
  const graph = new graphology.Graph({ type: 'directed', multi: false });

  // Map nodeId → { callerCount, calleeCount } to size nodes by degree.
  const inDegree  = new Map();
  const outDegree = new Map();
  for (const e of (rawEdges || [])) {
    outDegree.set(e.from, (outDegree.get(e.from) || 0) + 1);
    inDegree.set(e.to,   (inDegree.get(e.to)   || 0) + 1);
  }

  // Assign deterministic initial positions grouped by file.
  // Files get a slot on a circle; functions within a file spread around it.
  const fileSlots = new Map(); // file → { cx, cy, count, angle }
  for (const n of (rawNodes || [])) {
    if (!fileSlots.has(n.file)) fileSlots.set(n.file, { count: 0 });
    fileSlots.get(n.file).count++;
  }
  const fileList = [...fileSlots.keys()];
  const nFiles   = fileList.length;
  const outerR   = Math.max(400, nFiles * 60);
  fileList.forEach((file, fi) => {
    const angle = (fi / nFiles) * 2 * Math.PI;
    const slot = fileSlots.get(file);
    slot.cx    = outerR * Math.cos(angle);
    slot.cy    = outerR * Math.sin(angle);
    slot.angle = angle;
    slot.idx   = 0;
  });

  for (const n of (rawNodes || [])) {
    const slot = fileSlots.get(n.file);
    const localR = Math.max(30, slot.count * 15 / (2 * Math.PI));
    const localA = slot.angle + (slot.idx / slot.count) * 2 * Math.PI;
    slot.idx++;
    const deg = (inDegree.get(n.id) || 0) + (outDegree.get(n.id) || 0);
    graph.addNode(n.id, {
      label:     n.name,
      x:         slot.cx + localR * Math.cos(localA),
      y:         slot.cy + localR * Math.sin(localA),
      size:      3 + Math.min(Math.log1p(deg) * 1.2, 8),
      color:     fileColor(n.file),
      // Store metadata for reducers / tooltips.
      _file:      n.file,
      _lang:      n.language,
      _startLine: n.start_line,
      _endLine:   n.end_line,
      _in:        inDegree.get(n.id)  || 0,
      _out:       outDegree.get(n.id) || 0,
    });
  }

  for (const e of (rawEdges || [])) {
    if (!graph.hasNode(e.from) || !graph.hasNode(e.to)) continue;
    const eid = e.id || `${e.from}>${e.to}`;
    if (!graph.hasEdge(eid)) {
      graph.addEdgeWithKey(eid, e.from, e.to, {
        color: 'rgba(80,100,120,0.35)',
        size:  1,
        _kind: e.kind,
      });
    }
  }

  nodeCountEl.textContent = `${graph.order} nodes · ${graph.size} edges`;
  stNodes.textContent     = `nodes: ${graph.order}`;
  stEdges.textContent     = `edges: ${graph.size}`;

  // ── 3. Create Sigma renderer ───────────────────────────────────────────
  loadingText.textContent = 'Rendering…';

  const lod = new LODManager();
  let lodNodes = [];   // cached { id, x, y, degree } array rebuilt on layout stop

  function rebuildLodCache() {
    lodNodes = [];
    graph.forEachNode((id, attrs) => {
      lodNodes.push({ id, x: attrs.x, y: attrs.y, degree: graph.degree(id) });
    });
  }
  rebuildLodCache();

  // Declared with let so edgeReducer/nodeReducer can safely reference it
  // before the assignment completes (renderer is undefined, not TDZ).
  let renderer;
  renderer = new Sigma(graph, sigmaEl, {
    renderEdgeLabels:     false,
    defaultEdgeType:      'arrow',
    defaultNodeColor:     '#58a6ff',
    labelFont:            'system-ui, sans-serif',
    labelSize:            12,
    labelWeight:          '500',
    labelColor:           { color: '#c9d1d9' },
    minCameraRatio:       0.02,
    maxCameraRatio:       20,

    nodeReducer: (node, data) => {
      const result = { ...data };
      const ratio  = renderer ? renderer.getCamera().ratio : 1;

      // LOD: hide nodes that were merged into a cluster representative.
      if (lod.hiddenNodes.has(node)) {
        result.hidden = true;
        return result;
      }

      // Scale node size inversely with zoom so nodes stay visually small
      // when zoomed out and don't turn into giant blobs.
      // At ratio=1 (normal): size unchanged.  ratio=4 (far out): size/2.
      const zoomScale = 1 / Math.sqrt(Math.max(1, ratio));
      result.size = Math.max(2, data.size * zoomScale);

      // Cluster representative: show count, inflate slightly.
      const cs = lod.clusterSize.get(node);
      if (cs) {
        result.size  = result.size + Math.log1p(cs) * 1.5;
        result.label = `${data.label} (+${cs - 1})`;
      }

      // Hide labels when zoomed out — only show them when close enough.
      // ratio < 0.5 means zoomed in (1px graph = >2px screen).
      if (ratio > 0.45 && !cs) {
        result.label = '';
      }

      // Hover dimming: dim everything not in the hovered subtree.
      if (hoveredNode && hoveredSubtree && !hoveredSubtree.nodes.has(node)) {
        result.color = '#1e2a38';
        result.label = '';
        result.size  = Math.max(result.size * 0.5, 1.5);
        return result;
      }

      // Selected node: ring highlight.
      if (node === selectedNode) {
        result.highlighted = true;
      }

      // Hovered node itself: brighten.
      if (node === hoveredNode) {
        result.size  = result.size * 1.6;
        result.color = '#ffffff';
      }

      return result;
    },

    edgeReducer: (edge, data) => {
      const result = { ...data };
      // renderer is not yet assigned when Sigma calls this during construction,
      // so we read the camera lazily via the sigma instance reference below.
      const ratio  = renderer ? renderer.getCamera().ratio : 1;

      // Fade edges when very zoomed out.
      if (ratio > LOD_THRESHOLD * 2) {
        result.hidden = true;
        return result;
      }

      if (hoveredNode && hoveredSubtree) {
        if (hoveredSubtree.edges.has(edge)) {
          const src = graph.source(edge);
          result.color = src === hoveredNode ? '#58a6ff' : '#3fb950';
          result.size  = 2;
        } else {
          result.color = 'rgba(40,50,60,0.2)';
          result.size  = 0.5;
        }
      }
      return result;
    },
  });

  // ── 4. LOD on camera move ──────────────────────────────────────────────
  let lodDebounce = null;
  renderer.getCamera().on('updated', state => {
    stZoom.textContent = `zoom: ${(1 / state.ratio).toFixed(2)}`;

    if (lodNodes.length > 0) {
      clearTimeout(lodDebounce);
      lodDebounce = setTimeout(() => {
        lod.recompute(lodNodes, state.ratio);
        renderer.refresh();
      }, 60);
    }
  });

  // ── 5. Force-Atlas2 layout ─────────────────────────────────────────────
  loadingText.textContent = 'Computing layout…';

  // ── d3-force layout ───────────────────────────────────────────────────
  // We drive d3-force manually with requestAnimationFrame (never call
  // .restart() — that starts d3's internal timer which conflicts).

  const btnLayout = document.getElementById('btn-layout');
  let d3Running   = false;
  let d3RafId     = null;

  // Build d3 node/link arrays once; reuse across start/stop.
  const d3Nodes = [];
  const d3Links = [];
  {
    const idxById = {};
    graph.forEachNode((id, attrs) => {
      idxById[id] = d3Nodes.length;
      d3Nodes.push({ id, x: attrs.x, y: attrs.y });
    });
    graph.forEachEdge((_, __, src, tgt) => {
      if (idxById[src] !== undefined && idxById[tgt] !== undefined)
        d3Links.push({ source: idxById[src], target: idxById[tgt] });
    });
  }

  // Create simulation in stopped state — we tick() manually.
  // Charge strength scales with node count so large graphs spread out more.
  const chargeStrength = -Math.max(150, Math.min(600, graph.order * 1.2));
  const linkDist       = Math.max(60, Math.min(200, 2000 / Math.sqrt(graph.order + 1)));

  const d3Sim = d3.forceSimulation(d3Nodes)
    .force('link',    d3.forceLink(d3Links).distance(linkDist).strength(0.3))
    .force('charge',  d3.forceManyBody().strength(chargeStrength).theta(0.8))
    .force('center',  d3.forceCenter(0, 0).strength(0.04))
    .force('collide', d3.forceCollide(linkDist * 0.4).strength(0.7))
    .alphaDecay(0.012)
    .velocityDecay(0.35)
    .stop();  // IMPORTANT: never call .restart(); drive via tick()

  function syncPositions() {
    for (const dn of d3Nodes) {
      graph.setNodeAttribute(dn.id, 'x', dn.x);
      graph.setNodeAttribute(dn.id, 'y', dn.y);
    }
  }

  function layoutFrame() {
    if (!d3Running) return;
    d3Sim.tick();
    syncPositions();
    renderer.refresh();
    if (d3Sim.alpha() < d3Sim.alphaMin()) {
      stopLayout();
    } else {
      d3RafId = requestAnimationFrame(layoutFrame);
    }
  }

  function startLayout() {
    if (d3Running) return;
    d3Running = true;
    d3Sim.alpha(0.5);   // reheat without .restart()
    btnLayout.textContent = '⏸ Stop';
    btnLayout.classList.add('active');
    d3RafId = requestAnimationFrame(layoutFrame);
  }

  function stopLayout() {
    d3Running = false;
    if (d3RafId) { cancelAnimationFrame(d3RafId); d3RafId = null; }
    btnLayout.textContent = '▶ Layout';
    btnLayout.classList.remove('active');
    rebuildLodCache();
    lod.recompute(lodNodes, renderer.getCamera().ratio);
    renderer.refresh();
  }

  btnLayout.addEventListener('click', () => d3Running ? stopLayout() : startLayout());

  // Quick synchronous pre-layout (no animation, no RAF).
  loadingText.textContent = 'Applying initial layout…';
  d3Sim.tick(graph.order < 200 ? 400 : graph.order < 800 ? 150 : 80);
  syncPositions();
  rebuildLodCache();

  // Hide loading overlay.
  loadingEl.classList.add('hidden');
  setTimeout(() => { loadingEl.style.display = 'none'; }, 450);

  // ── 6. Fit to screen ──────────────────────────────────────────────────
  document.getElementById('btn-fit').addEventListener('click', () => {
    renderer.getCamera().animatedReset();
  });

  // ── 7. Hover interactions ─────────────────────────────────────────────
  renderer.on('enterNode', ({ node, event }) => {
    hoveredNode    = node;
    hoveredSubtree = computeSubtree(graph, node);
    renderer.refresh();

    const attrs = graph.getNodeAttributes(node);
    ttName.textContent  = attrs.label;
    ttFile.textContent  = attrs._file;
    ttStats.textContent = `callers: ${attrs._in}  callees: ${attrs._out}`;
    tooltip.classList.add('visible');
    moveTooltip(event);
    stHover.textContent = attrs.label;
  });

  renderer.on('leaveNode', () => {
    hoveredNode    = null;
    hoveredSubtree = null;
    renderer.refresh();
    tooltip.classList.remove('visible');
    stHover.textContent = '';
  });

  renderer.on('moveBody', ({ event }) => {
    if (tooltip.classList.contains('visible')) moveTooltip(event);
  });

  function moveTooltip(event) {
    const x = event.x + 14;
    const y = event.y + 14;
    tooltip.style.left = Math.min(x, window.innerWidth  - 290) + 'px';
    tooltip.style.top  = Math.min(y, window.innerHeight - 90)  + 'px';
  }

  // ── 8. Click → open code panel ────────────────────────────────────────
  renderer.on('clickNode', ({ node }) => {
    selectedNode = node;
    renderer.refresh();
    openCodePanel(node);
  });

  renderer.on('clickStage', () => {
    selectedNode = null;
    renderer.refresh();
  });

  // ── 9. Code panel ─────────────────────────────────────────────────────
  document.getElementById('panel-close').addEventListener('click', () => {
    codePanel.classList.remove('open');
    selectedNode = null;
    renderer.refresh();
  });

  async function openCodePanel(nodeId) {
    const attrs = graph.getNodeAttributes(nodeId);
    codePanel.classList.add('open');

    panelFnName.textContent   = attrs.label;
    panelFilePath.textContent = attrs._file;

    // Meta chips
    panelMeta.innerHTML = `
      <span class="meta-chip">lang: <b>${attrs._lang || '?'}</b></span>
      <span class="meta-chip">lines: <b>${attrs._startLine}–${attrs._endLine}</b></span>
      <span class="meta-chip">callers: <b>${attrs._in}</b></span>
      <span class="meta-chip">callees: <b>${attrs._out}</b></span>
    `;

    codeOutput.innerHTML = '<div class="code-loading">Loading…</div>';

    let data;
    try {
      const res = await fetch(`/api/file?path=${encodeURIComponent(attrs._file)}`);
      if (!res.ok) {
        codeOutput.innerHTML = `<div class="code-loading">Could not load file: ${await res.text()}</div>`;
        return;
      }
      data = await res.json();
    } catch (e) {
      codeOutput.innerHTML = `<div class="code-loading">Error: ${e.message}</div>`;
      return;
    }

    renderCode(data.content, data.language, attrs._startLine, attrs._endLine);
  }

  function renderCode(content, language, fnStart, fnEnd) {
    // Run highlight.js on the full file content.
    let highlighted;
    try {
      highlighted = hljs.highlight(content, { language, ignoreIllegals: true }).value;
    } catch (_) {
      highlighted = hljs.highlightAuto(content).value;
    }

    // Split highlighted HTML into lines, preserving open <span> tags
    // across line breaks so the colouring never breaks.
    const lines = splitHighlightedLines(highlighted);

    const rows = lines.map((lineHtml, i) => {
      const lineNo = i + 1;
      const isFn   = lineNo >= fnStart && lineNo <= fnEnd;
      let cls = 'code-row';
      if (isFn) {
        cls += ' fn-line';
        if (lineNo === fnStart) cls += ' fn-line-start';
        if (lineNo === fnEnd)   cls += ' fn-line-end';
      }
      return `<div class="${cls}" data-line="${lineNo}">` +
             `<span class="line-num">${lineNo}</span>` +
             `<span class="line-code">${lineHtml || ' '}</span>` +
             `</div>`;
    });

    codeOutput.innerHTML = `<div class="code-table">${rows.join('')}</div>`;

    // Scroll to the first function line.
    const target = codeOutput.querySelector('.fn-line-start');
    if (target) {
      requestAnimationFrame(() => {
        const scrollTop = target.offsetTop - codeScroll.clientHeight * 0.3;
        codeScroll.scrollTop = Math.max(0, scrollTop);
      });
    }
  }

  // ── 10. Split highlighted HTML into per-line arrays ────────────────────
  // This parser re-opens any unclosed <span> tags on the next line so
  // multi-line strings / comments stay properly coloured.
  function splitHighlightedLines(html) {
    const lines = [];
    let current = '';
    let openTags = [];   // stack of open span class-stacks
    let i = 0;

    while (i < html.length) {
      if (html[i] === '\n') {
        lines.push(current);
        // Re-open any unclosed spans on next line.
        current = openTags.map(t => `<span class="${t}">`).join('');
        i++;
        continue;
      }
      if (html[i] === '<') {
        const tagEnd = html.indexOf('>', i);
        if (tagEnd === -1) { current += html[i++]; continue; }
        const tag = html.slice(i, tagEnd + 1);
        if (tag.startsWith('<span')) {
          const m = tag.match(/class="([^"]*)"/);
          if (m) openTags.push(m[1]);
        } else if (tag === '</span>') {
          openTags.pop();
        }
        current += tag;
        i = tagEnd + 1;
        continue;
      }
      current += html[i++];
    }
    if (current) lines.push(current);

    // Close any open spans on the last pushed line.
    if (lines.length && openTags.length) {
      lines[lines.length - 1] += '</span>'.repeat(openTags.length);
    }
    return lines;
  }

  // ── 11. Search ─────────────────────────────────────────────────────────
  searchInput.addEventListener('input', () => {
    const q = searchInput.value.trim().toLowerCase();
    if (!q) { searchResults.classList.remove('open'); return; }

    const matches = [];
    graph.forEachNode((id, attrs) => {
      if (attrs.label.toLowerCase().includes(q) || attrs._file.toLowerCase().includes(q)) {
        matches.push({ id, label: attrs.label, file: attrs._file });
      }
    });

    if (!matches.length) { searchResults.classList.remove('open'); return; }

    searchResults.innerHTML = matches.slice(0, 20).map(m =>
      `<div class="sr-item" data-id="${m.id}">
        <div class="sr-name">${esc(m.label)}</div>
        <div class="sr-file">${esc(m.file)}</div>
      </div>`
    ).join('');
    searchResults.classList.add('open');
  });

  searchResults.addEventListener('click', e => {
    const item = e.target.closest('.sr-item');
    if (!item) return;
    const id = item.dataset.id;
    searchResults.classList.remove('open');
    searchInput.value = '';
    navigateTo(id);
  });

  document.addEventListener('click', e => {
    if (!searchResults.contains(e.target) && e.target !== searchInput) {
      searchResults.classList.remove('open');
    }
  });

  function navigateTo(nodeId) {
    const attrs = graph.getNodeAttributes(nodeId);
    renderer.getCamera().animate(
      { x: attrs.x, y: attrs.y, ratio: 0.3 },
      { duration: 500 }
    );
    selectedNode = nodeId;
    renderer.refresh();
    openCodePanel(nodeId);
  }

  // ── 12. Direction filters ──────────────────────────────────────────────
  // These buttons highlight only the upstream or downstream sub-tree of
  // the currently selected node.
  function computeDirectionalSubtree(nodeId, direction) {
    if (!nodeId) return null;
    const nodes = new Set([nodeId]);
    const edges = new Set();
    const queue = [nodeId];
    const visited = new Set([nodeId]);

    while (queue.length) {
      const cur = queue.shift();
      const edgeIter = direction === 'callers'
        ? graph.inEdges(cur)
        : graph.outEdges(cur);
      edgeIter.forEach(eid => {
        const neighbor = direction === 'callers' ? graph.source(eid) : graph.target(eid);
        edges.add(eid);
        nodes.add(neighbor);
        if (!visited.has(neighbor)) { visited.add(neighbor); queue.push(neighbor); }
      });
    }
    return { nodes, edges };
  }

  let directionFilter = null;  // 'callers' | 'callees' | null

  document.getElementById('btn-callers-only').addEventListener('click', function() {
    if (directionFilter === 'callers') {
      directionFilter = null;
      this.classList.remove('active');
    } else {
      directionFilter = 'callers';
      this.classList.add('active');
      document.getElementById('btn-callees-only').classList.remove('active');
    }
    applyDirectionFilter();
  });

  document.getElementById('btn-callees-only').addEventListener('click', function() {
    if (directionFilter === 'callees') {
      directionFilter = null;
      this.classList.remove('active');
    } else {
      directionFilter = 'callees';
      this.classList.add('active');
      document.getElementById('btn-callers-only').classList.remove('active');
    }
    applyDirectionFilter();
  });

  function applyDirectionFilter() {
    if (!directionFilter || !selectedNode) {
      graph.forEachNode(id => graph.setNodeAttribute(id, '_filtered', false));
    } else {
      const sub = computeDirectionalSubtree(selectedNode, directionFilter);
      graph.forEachNode(id => graph.setNodeAttribute(id, '_filtered', !sub.nodes.has(id)));
    }
    renderer.refresh();
  }

  // Integrate _filtered into nodeReducer (patch done below).
  const origNodeReducer = renderer.settings.nodeReducer;
  renderer.setSetting('nodeReducer', (node, data) => {
    const result = origNodeReducer(node, data);
    if (graph.getNodeAttribute(node, '_filtered')) {
      result.hidden = true;
    }
    return result;
  });

  // ── Panel resize ───────────────────────────────────────────────────────
  const resizeHandle = document.getElementById('panel-resize');
  let resizing = false;
  let resizeStartX = 0;
  let resizeStartW = 0;

  resizeHandle.addEventListener('mousedown', e => {
    e.preventDefault();
    resizing    = true;
    resizeStartX = e.clientX;
    resizeStartW = codePanel.offsetWidth;
    resizeHandle.classList.add('dragging');
    document.body.style.cursor    = 'ew-resize';
    document.body.style.userSelect = 'none';
  });

  document.addEventListener('mousemove', e => {
    if (!resizing) return;
    // Panel is on the right; dragging left = wider, right = narrower.
    const delta   = resizeStartX - e.clientX;
    const minW    = parseInt(getComputedStyle(codePanel).minWidth) || 260;
    const maxW    = Math.round(window.innerWidth * 0.9);
    const newW    = Math.min(maxW, Math.max(minW, resizeStartW + delta));
    codePanel.style.width = newW + 'px';
  });

  document.addEventListener('mouseup', () => {
    if (!resizing) return;
    resizing = false;
    resizeHandle.classList.remove('dragging');
    document.body.style.cursor     = '';
    document.body.style.userSelect = '';
  });

  // ── Utility ────────────────────────────────────────────────────────────
  function esc(s) {
    return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }

  } catch (e) {
    // Any uncaught error during init shows up in the loading overlay.
    loadingText.textContent = 'Init error: ' + e.message;
    console.error('funcgraph init error', e);
  }
});
