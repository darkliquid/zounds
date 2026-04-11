const state = {
  tag: "",
  query: "",
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

  for (const cluster of state.clusters) {
    const li = document.createElement("li");
    li.textContent = `${cluster.Label} (${(cluster.Samples || []).length})`;
    root.appendChild(li);
  }
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
  search.addEventListener("input", async (event) => {
    state.query = event.target.value.trim();
    await refreshSamples();
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
    loadJSON("/api/clusters"),
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
