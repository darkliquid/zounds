const state = {
  tag: "",
  query: "",
  projection: "tsne",
  samples: [],
  tags: [],
  clusters: [],
};

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

function renderClusters() {
  const root = document.getElementById("clusters");
  root.innerHTML = "";
  const svg = document.getElementById("cluster-map");
  svg.innerHTML = "";
  const maxSize = state.clusters.reduce((max, cluster) => Math.max(max, (cluster.Samples || []).length), 1);
  const positions = normalizeClusterPositions(state.clusters);

  state.clusters.forEach((cluster, index) => {
    const size = (cluster.Samples || []).length;
    const li = document.createElement("li");
    li.textContent = `${cluster.Label} (${size})`;
    root.appendChild(li);

    const radius = 18 + (size / maxSize) * 26;
    const point = positions[index];
    const cx = point.x;
    const cy = point.y;

    const bubble = document.createElementNS("http://www.w3.org/2000/svg", "circle");
    bubble.setAttribute("cx", cx);
    bubble.setAttribute("cy", cy);
    bubble.setAttribute("r", radius);
    bubble.setAttribute("class", "cluster-bubble");
    svg.appendChild(bubble);

    const label = document.createElementNS("http://www.w3.org/2000/svg", "text");
    label.setAttribute("x", cx);
    label.setAttribute("y", cy);
    label.setAttribute("class", "cluster-label");
    label.textContent = cluster.Label;
    svg.appendChild(label);
  });
}

function normalizeClusterPositions(clusters) {
  if (!clusters.length) {
    return [];
  }

  const allHaveProjection = clusters.every((cluster) => Number.isFinite(cluster.x) && Number.isFinite(cluster.y));
  if (!allHaveProjection) {
    return clusters.map((_, index) => {
      const col = index % 3;
      const row = Math.floor(index / 3);
      return { x: 48 + col * 86, y: 48 + row * 86 };
    });
  }

  const xs = clusters.map((cluster) => cluster.x);
  const ys = clusters.map((cluster) => cluster.y);
  const minX = Math.min(...xs);
  const maxX = Math.max(...xs);
  const minY = Math.min(...ys);
  const maxY = Math.max(...ys);
  const spanX = maxX - minX || 1;
  const spanY = maxY - minY || 1;

  return clusters.map((cluster) => ({
    x: 28 + ((cluster.x - minX) / spanX) * 224,
    y: 28 + ((cluster.y - minY) / spanY) * 184,
  }));
}

function renderSamples() {
  const root = document.getElementById("samples");
  root.innerHTML = "";

  for (const sample of state.samples) {
    const li = document.createElement("li");
    li.className = "sample-row";

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

    const preview = document.createElement("button");
    preview.type = "button";
    preview.textContent = "Preview";
    preview.addEventListener("click", () => showPreview(sample));

    li.appendChild(meta);
    li.appendChild(preview);
    root.appendChild(li);
  }
}

function showPreview(sample) {
  setHidden("preview-empty", true);
  setHidden("preview", false);

  document.getElementById("preview-title").textContent = sample.FileName;
  document.getElementById("preview-path").textContent = sample.Path;

  const tagRoot = document.getElementById("preview-tags");
  tagRoot.innerHTML = "";
  for (const tag of sample.tags || []) {
    const span = document.createElement("span");
    span.className = "tag";
    span.textContent = tag.NormalizedName;
    tagRoot.appendChild(span);
  }

  const player = document.getElementById("player");
  player.src = `/api/samples/${sample.ID}/audio`;
  player.load();
}

async function main() {
  const search = document.getElementById("search");
  const projection = document.getElementById("projection");
  search.addEventListener("input", async (event) => {
    state.query = event.target.value.trim();
    await refreshSamples();
  });
  projection.addEventListener("change", async (event) => {
    state.projection = event.target.value;
    state.clusters = await loadJSON(`/api/clusters?projection=${encodeURIComponent(state.projection)}`);
    renderClusters();
  });

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

  renderTags();
  renderClusters();
  await refreshSamples();
}

main().catch((error) => {
  console.error(error);
});
