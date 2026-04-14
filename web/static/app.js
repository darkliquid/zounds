const state = {
  browser: "search",
  tag: "",
  query: "",
  projection: "tsne",
  samples: [],
  tags: [],
  clusters: [],
  activeClusterId: null,
  activeClusterSampleId: null,
  clusterViewport: {
    scale: 1,
    x: 0,
    y: 0,
  },
};

const CLUSTER_VIEWBOX = { width: 1000, height: 700, padding: 60 };

async function loadJSON(path) {
  const response = await fetch(path);
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json();
}

function setHidden(id, hidden) {
  document.getElementById(id).classList.toggle("hidden", hidden);
}

function sampleURL() {
  const params = new URLSearchParams();
  if (state.tag) {
    params.set("tag", state.tag);
  }
  if (state.query) {
    params.set("q", state.query);
  }
  const query = params.toString();
  return query ? `/api/samples?${query}` : "/api/samples";
}

async function refreshSamples() {
  state.samples = await loadJSON(sampleURL());
  renderSamples();
  renderSummary();
}

async function refreshClusters() {
  state.clusters = await loadJSON(`/api/clusters?projection=${encodeURIComponent(state.projection)}`);
  if (!state.clusters.some((cluster) => cluster.ID === state.activeClusterId)) {
    state.activeClusterId = state.clusters[0]?.ID ?? null;
    state.activeClusterSampleId = null;
  }
  resetClusterViewport();
  renderClusters();
}

function renderSummary() {
  const summary = document.getElementById("summary");
  summary.textContent = `${state.samples.length} sample${state.samples.length === 1 ? "" : "s"} shown`;

  const filter = document.getElementById("active-filter");
  if (!state.tag && !state.query) {
    filter.textContent = "";
    filter.classList.add("hidden");
    return;
  }

  const parts = [];
  if (state.tag) {
    parts.push(`tag: ${state.tag}`);
  }
  if (state.query) {
    parts.push(`search: ${state.query}`);
  }
  filter.textContent = parts.join(" · ");
  filter.classList.remove("hidden");
}

function renderTags() {
  const root = document.getElementById("tags");
  root.innerHTML = "";
  const maxCount = state.tags.reduce((max, entry) => Math.max(max, entry.SampleCount), 1);

  for (const entry of state.tags) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = entry.Tag.NormalizedName === state.tag ? "tag active" : "tag";
    button.textContent = `${entry.Tag.NormalizedName} (${entry.SampleCount})`;
    const scale = 0.9 + (entry.SampleCount / maxCount) * 0.8;
    button.style.fontSize = `${scale}rem`;
    button.style.opacity = `${0.65 + (entry.SampleCount / maxCount) * 0.35}`;
    button.addEventListener("click", async () => {
      state.tag = entry.Tag.NormalizedName === state.tag ? "" : entry.Tag.NormalizedName;
      await refreshSamples();
      renderTags();
    });
    root.appendChild(button);
  }
}

function setBrowser(browser) {
  state.browser = browser;
  const searchActive = browser === "search";
  document.getElementById("tab-search").classList.toggle("active", searchActive);
  document.getElementById("tab-search").setAttribute("aria-selected", String(searchActive));
  document.getElementById("tab-clusters").classList.toggle("active", !searchActive);
  document.getElementById("tab-clusters").setAttribute("aria-selected", String(!searchActive));
  setHidden("browser-search", !searchActive);
  setHidden("browser-clusters", searchActive);
}

function renderClusters() {
  const root = document.getElementById("clusters");
  root.innerHTML = "";

  const svg = document.getElementById("cluster-map");
  svg.innerHTML = "";
  const scene = document.createElementNS("http://www.w3.org/2000/svg", "g");
  svg.appendChild(scene);

  const maxSize = state.clusters.reduce((max, cluster) => Math.max(max, (cluster.samples || []).length), 1);
  const layout = layoutClusters(state.clusters);

  for (const entry of layout) {
    const cluster = entry.cluster;
    const size = (cluster.samples || []).length;

    const listItem = document.createElement("li");
    const button = document.createElement("button");
    button.type = "button";
    button.className = cluster.ID === state.activeClusterId ? "cluster-item active" : "cluster-item";
    button.textContent = `${cluster.Label} (${size})`;
    button.addEventListener("click", () => selectCluster(cluster.ID));
    listItem.appendChild(button);
    root.appendChild(listItem);

    for (const member of entry.members) {
      const dot = document.createElementNS("http://www.w3.org/2000/svg", "circle");
      dot.setAttribute("cx", member.point.x);
      dot.setAttribute("cy", member.point.y);
      dot.setAttribute("r", member.sample.ID === state.activeClusterSampleId ? 7 : 4.5);
      dot.setAttribute("class", member.sample.ID === state.activeClusterSampleId ? "cluster-member active" : "cluster-member");
      dot.addEventListener("click", () => selectCluster(cluster.ID, member.sample.ID));

      const title = document.createElementNS("http://www.w3.org/2000/svg", "title");
      title.textContent = member.sample.FileName;
      dot.appendChild(title);
      scene.appendChild(dot);
    }

    const radius = 18 + (size / maxSize) * 26;
    const bubble = document.createElementNS("http://www.w3.org/2000/svg", "circle");
    bubble.setAttribute("cx", entry.center.x);
    bubble.setAttribute("cy", entry.center.y);
    bubble.setAttribute("r", radius);
    bubble.setAttribute("class", cluster.ID === state.activeClusterId ? "cluster-bubble active" : "cluster-bubble");
    bubble.addEventListener("click", () => selectCluster(cluster.ID));
    scene.appendChild(bubble);

    const label = document.createElementNS("http://www.w3.org/2000/svg", "text");
    label.setAttribute("x", entry.center.x);
    label.setAttribute("y", entry.center.y);
    label.setAttribute("class", "cluster-label");
    label.textContent = cluster.Label;
    scene.appendChild(label);
  }

  applyClusterViewport();
  renderClusterSelection();
}

function layoutClusters(clusters) {
  const bounds = projectionBounds(clusters);
  if (!bounds) {
    return clusters.map((cluster, index) => {
      const center = fallbackClusterPosition(index);
      return {
        cluster,
        center,
        members: (cluster.members || []).map((sample, memberIndex) => ({
          sample,
          point: fallbackMemberPosition(center, memberIndex),
        })),
      };
    });
  }

  return clusters.map((cluster, index) => {
    const center = Number.isFinite(cluster.x) && Number.isFinite(cluster.y)
      ? normalizeProjectionPoint(cluster, bounds)
      : fallbackClusterPosition(index);

    return {
      cluster,
      center,
      members: (cluster.members || []).map((sample, memberIndex) => ({
        sample,
        point: Number.isFinite(sample.x) && Number.isFinite(sample.y)
          ? normalizeProjectionPoint(sample, bounds)
          : fallbackMemberPosition(center, memberIndex),
      })),
    };
  });
}

function projectionBounds(clusters) {
  const projected = [];
  for (const cluster of clusters) {
    if (Number.isFinite(cluster.x) && Number.isFinite(cluster.y)) {
      projected.push(cluster);
    }
    for (const sample of cluster.members || []) {
      if (Number.isFinite(sample.x) && Number.isFinite(sample.y)) {
        projected.push(sample);
      }
    }
  }
  if (!projected.length) {
    return null;
  }

  const xs = projected.map((point) => point.x);
  const ys = projected.map((point) => point.y);
  return {
    minX: Math.min(...xs),
    maxX: Math.max(...xs),
    minY: Math.min(...ys),
    maxY: Math.max(...ys),
  };
}

function normalizeProjectionPoint(point, bounds) {
  const spanX = bounds.maxX - bounds.minX || 1;
  const spanY = bounds.maxY - bounds.minY || 1;
  return {
    x: CLUSTER_VIEWBOX.padding + ((point.x - bounds.minX) / spanX) * (CLUSTER_VIEWBOX.width - CLUSTER_VIEWBOX.padding * 2),
    y: CLUSTER_VIEWBOX.padding + ((point.y - bounds.minY) / spanY) * (CLUSTER_VIEWBOX.height - CLUSTER_VIEWBOX.padding * 2),
  };
}

function fallbackClusterPosition(index) {
  const columns = 3;
  const col = index % columns;
  const row = Math.floor(index / columns);
  return {
    x: 170 + col * 280,
    y: 150 + row * 210,
  };
}

function fallbackMemberPosition(center, index) {
  const angle = index * 0.7;
  const radius = 26 + Math.floor(index / 8) * 18;
  return {
    x: center.x + Math.cos(angle) * radius,
    y: center.y + Math.sin(angle) * radius,
  };
}

function renderClusterSelection() {
  const cluster = state.clusters.find((entry) => entry.ID === state.activeClusterId);
  setHidden("cluster-selection-empty", Boolean(cluster));
  setHidden("cluster-selection", !cluster);
  if (!cluster) {
    return;
  }

  const members = cluster.members || [];
  document.getElementById("cluster-title").textContent = cluster.Label;
  document.getElementById("cluster-summary").textContent = `${members.length} sample${members.length === 1 ? "" : "s"} in this cluster`;

  const root = document.getElementById("cluster-samples");
  root.innerHTML = "";
  for (const sample of members) {
    root.appendChild(createSampleRow(sample, sample.ID === state.activeClusterSampleId));
  }
}

function selectCluster(clusterId, sampleId = null) {
  state.activeClusterId = clusterId;
  state.activeClusterSampleId = sampleId;
  renderClusters();
}

function applyClusterViewport() {
  const scene = document.querySelector("#cluster-map g");
  if (!scene) {
    return;
  }
  scene.setAttribute("transform", `translate(${state.clusterViewport.x} ${state.clusterViewport.y}) scale(${state.clusterViewport.scale})`);
}

function resetClusterViewport() {
  state.clusterViewport.scale = 1;
  state.clusterViewport.x = 0;
  state.clusterViewport.y = 0;
  applyClusterViewport();
}

function bindClusterInteractions() {
  const svg = document.getElementById("cluster-map");
  let dragging = false;
  let lastX = 0;
  let lastY = 0;

  svg.addEventListener("pointerdown", (event) => {
    dragging = true;
    lastX = event.clientX;
    lastY = event.clientY;
    svg.setPointerCapture(event.pointerId);
  });

  svg.addEventListener("pointermove", (event) => {
    if (!dragging) {
      return;
    }
    state.clusterViewport.x += event.clientX - lastX;
    state.clusterViewport.y += event.clientY - lastY;
    lastX = event.clientX;
    lastY = event.clientY;
    applyClusterViewport();
  });

  const stopDragging = (event) => {
    if (!dragging) {
      return;
    }
    dragging = false;
    if (svg.hasPointerCapture(event.pointerId)) {
      svg.releasePointerCapture(event.pointerId);
    }
  };
  svg.addEventListener("pointerup", stopDragging);
  svg.addEventListener("pointercancel", stopDragging);

  svg.addEventListener("wheel", (event) => {
    event.preventDefault();
    const rect = svg.getBoundingClientRect();
    const focusX = event.clientX - rect.left;
    const focusY = event.clientY - rect.top;
    const currentScale = state.clusterViewport.scale;
    const nextScale = Math.max(0.6, Math.min(6, currentScale * (event.deltaY < 0 ? 1.12 : 0.88)));
    const ratio = nextScale / currentScale;

    state.clusterViewport.x = focusX - (focusX - state.clusterViewport.x) * ratio;
    state.clusterViewport.y = focusY - (focusY - state.clusterViewport.y) * ratio;
    state.clusterViewport.scale = nextScale;
    applyClusterViewport();
  }, { passive: false });
}

function createSampleRow(sample, highlighted = false) {
  const li = document.createElement("li");
  li.className = highlighted ? "sample-row active" : "sample-row";

  const meta = document.createElement("div");
  meta.className = "sample-meta";

  const title = document.createElement("strong");
  title.textContent = sample.FileName;
  meta.appendChild(title);

  const path = document.createElement("div");
  path.className = "sample-path";
  path.textContent = sample.Path;
  meta.appendChild(path);

  const tagWrap = document.createElement("div");
  tagWrap.className = "tag-cloud";
  for (const tag of sample.tags || []) {
    const span = document.createElement("span");
    span.className = "tag";
    span.textContent = tag.NormalizedName;
    tagWrap.appendChild(span);
  }
  meta.appendChild(tagWrap);

  const player = document.createElement("audio");
  player.controls = true;
  player.preload = "none";
  player.src = `/api/samples/${sample.ID}/audio`;
  meta.appendChild(player);

  li.appendChild(meta);
  return li;
}

function renderSamples() {
  const root = document.getElementById("samples");
  root.innerHTML = "";

  for (const sample of state.samples) {
    root.appendChild(createSampleRow(sample));
  }
}

function wireTabs() {
  document.getElementById("tab-search").addEventListener("click", () => setBrowser("search"));
  document.getElementById("tab-clusters").addEventListener("click", () => setBrowser("clusters"));
}

async function main() {
  const search = document.getElementById("search");
  const projection = document.getElementById("projection");

  wireTabs();
  bindClusterInteractions();

  search.addEventListener("input", async (event) => {
    state.query = event.target.value.trim();
    await refreshSamples();
  });
  projection.addEventListener("change", async (event) => {
    state.projection = event.target.value;
    await refreshClusters();
  });
  document.getElementById("reset-cluster-view").addEventListener("click", () => resetClusterViewport());

  document.getElementById("clear-filters").addEventListener("click", async () => {
    state.tag = "";
    state.query = "";
    search.value = "";
    await refreshSamples();
    renderTags();
  });

  const [tags, clusters] = await Promise.all([
    loadJSON("/api/tags"),
    loadJSON(`/api/clusters?projection=${encodeURIComponent(state.projection)}`),
  ]);
  state.tags = tags;
  state.clusters = clusters;
  state.activeClusterId = state.clusters[0]?.ID ?? null;

  setBrowser("search");
  renderTags();
  renderClusters();
  await refreshSamples();
}

main().catch((error) => {
  console.error(error);
});
