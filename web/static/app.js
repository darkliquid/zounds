const state = {
  browser: "search",
  tag: "",
  query: "",
  similarityMetric: "acoustic",
  similarReference: null,
  facets: {
    type: "",
    bpmMin: "",
    bpmMax: "",
    key: "",
    chord: "",
    durationMin: "",
    durationMax: "",
  },
  projection: "tsne",
  samples: [],
  tags: [],
  clusters: [],
  savingTags: new Set(),
  activeClusterId: null,
  activeClusterSampleId: null,
  clusterViewport: {
    scale: 1,
    x: 0,
    y: 0,
  },
};

const CLUSTER_VIEWBOX = { width: 1000, height: 700, padding: 60 };

function getSampleID(s) {
  return s.id ?? s.ID;
}

function getSampleFileName(s) {
  return s.file_name ?? s.FileName;
}

function getSamplePath(s) {
  return s.path ?? s.Path;
}

function getTagName(t) {
  return t.normalized_name ?? t.NormalizedName ?? t.name ?? t.Name;
}

function getClusterID(c) {
  return c.id ?? c.ID;
}

function getClusterLabel(c) {
  return c.label ?? c.Label;
}

function getTagUsageName(entry) {
  return entry.tag?.normalized_name ?? entry.Tag?.NormalizedName ?? entry.tag?.name ?? entry.Tag?.Name ?? "";
}

function getTagUsageCount(entry) {
  return entry.sample_count ?? entry.SampleCount ?? 0;
}

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
  if (state.similarReference) {
    const params = new URLSearchParams();
    if (state.similarityMetric) {
      params.set("metric", state.similarityMetric);
    }
    const query = params.toString();
    return query
      ? `/api/samples/${encodeURIComponent(state.similarReference.id)}/similar?${query}`
      : `/api/samples/${encodeURIComponent(state.similarReference.id)}/similar`;
  }

  const params = new URLSearchParams();
  if (state.tag) {
    params.set("tag", state.tag);
  }
  if (state.query) {
    params.set("q", state.query);
  }
  if (state.facets.type) {
    params.set("type", state.facets.type);
  }
  if (state.facets.bpmMin) {
    params.set("bpm_min", state.facets.bpmMin);
  }
  if (state.facets.bpmMax) {
    params.set("bpm_max", state.facets.bpmMax);
  }
  if (state.facets.key) {
    params.set("key", state.facets.key);
  }
  if (state.facets.chord) {
    params.set("chord", state.facets.chord);
  }
  if (state.facets.durationMin) {
    params.set("duration_min", state.facets.durationMin);
  }
  if (state.facets.durationMax) {
    params.set("duration_max", state.facets.durationMax);
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
  if (!state.clusters.some((cluster) => getClusterID(cluster) === state.activeClusterId)) {
    state.activeClusterId = state.clusters[0] ? getClusterID(state.clusters[0]) : null;
    state.activeClusterSampleId = null;
  }
  resetClusterViewport();
  renderClusters();
}

function renderSummary() {
  const summary = document.getElementById("summary");
  summary.textContent = `${state.samples.length} sample${state.samples.length === 1 ? "" : "s"} shown`;

  const filter = document.getElementById("active-filter");
  const parts = [];
  if (state.similarReference) {
    parts.push(`similar to: ${state.similarReference.fileName} (${state.similarityMetric})`);
  }
  if (state.tag) {
    parts.push(`tag: ${state.tag}`);
  }
  if (state.query) {
    parts.push(`search: ${state.query}`);
  }
  if (state.facets.type) {
    parts.push(`type: ${state.facets.type}`);
  }
  if (state.facets.bpmMin || state.facets.bpmMax) {
    parts.push(`BPM: ${state.facets.bpmMin || "0"}-${state.facets.bpmMax || "∞"}`);
  }
  if (state.facets.key) {
    parts.push(`key: ${state.facets.key}`);
  }
  if (state.facets.chord) {
    parts.push(`chord: ${state.facets.chord}`);
  }
  if (state.facets.durationMin || state.facets.durationMax) {
    parts.push(`duration: ${state.facets.durationMin || "0"}-${state.facets.durationMax || "∞"}s`);
  }

  if (parts.length === 0) {
    filter.textContent = "";
    filter.classList.add("hidden");
    return;
  }
  filter.textContent = parts.join(" · ");
  filter.classList.remove("hidden");
}

function renderTags() {
  const root = document.getElementById("tags");
  root.innerHTML = "";
  const maxCount = state.tags.reduce((max, entry) => Math.max(max, getTagUsageCount(entry)), 1);

  for (const entry of state.tags) {
    const tagName = getTagUsageName(entry);
    const count = getTagUsageCount(entry);
    const button = document.createElement("button");
    button.type = "button";
    button.className = tagName === state.tag ? "tag active" : "tag";
    button.textContent = `${tagName} (${count})`;
    const scale = 0.9 + (count / maxCount) * 0.8;
    button.style.fontSize = `${scale}rem`;
    button.style.opacity = `${0.65 + (count / maxCount) * 0.35}`;
    button.addEventListener("click", async () => {
      state.tag = tagName === state.tag ? "" : tagName;
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
    const clusterID = getClusterID(cluster);
    const clusterLabel = getClusterLabel(cluster);
    const size = (cluster.samples || []).length;

    const listItem = document.createElement("li");
    const button = document.createElement("button");
    button.type = "button";
    button.className = clusterID === state.activeClusterId ? "cluster-item active" : "cluster-item";
    button.textContent = `${clusterLabel} (${size})`;
    button.addEventListener("click", () => selectCluster(clusterID));
    listItem.appendChild(button);
    root.appendChild(listItem);

    for (const member of entry.members) {
      const memberSampleID = getSampleID(member.sample);
      const dot = document.createElementNS("http://www.w3.org/2000/svg", "circle");
      dot.setAttribute("cx", member.point.x);
      dot.setAttribute("cy", member.point.y);
      dot.setAttribute("r", memberSampleID === state.activeClusterSampleId ? 7 : 4.5);
      dot.setAttribute("class", memberSampleID === state.activeClusterSampleId ? "cluster-member active" : "cluster-member");
      dot.addEventListener("click", () => selectCluster(clusterID, memberSampleID));

      const title = document.createElementNS("http://www.w3.org/2000/svg", "title");
      title.textContent = getSampleFileName(member.sample);
      dot.appendChild(title);
      scene.appendChild(dot);
    }

    const radius = 18 + (size / maxSize) * 26;
    const bubble = document.createElementNS("http://www.w3.org/2000/svg", "circle");
    bubble.setAttribute("cx", entry.center.x);
    bubble.setAttribute("cy", entry.center.y);
    bubble.setAttribute("r", radius);
    bubble.setAttribute("class", clusterID === state.activeClusterId ? "cluster-bubble active" : "cluster-bubble");
    bubble.addEventListener("click", () => selectCluster(clusterID));
    scene.appendChild(bubble);

    const label = document.createElementNS("http://www.w3.org/2000/svg", "text");
    label.setAttribute("x", entry.center.x);
    label.setAttribute("y", entry.center.y);
    label.setAttribute("class", "cluster-label");
    label.textContent = clusterLabel;
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
  const cluster = state.clusters.find((entry) => getClusterID(entry) === state.activeClusterId);
  setHidden("cluster-selection-empty", Boolean(cluster));
  setHidden("cluster-selection", !cluster);
  if (!cluster) {
    return;
  }

  const members = cluster.members || [];
  document.getElementById("cluster-title").textContent = getClusterLabel(cluster);
  document.getElementById("cluster-summary").textContent = `${members.length} sample${members.length === 1 ? "" : "s"} in this cluster`;

  const root = document.getElementById("cluster-samples");
  root.innerHTML = "";
  for (const member of members) {
    const s = member.sample || member;
    root.appendChild(createSampleRow(s, getSampleID(s) === state.activeClusterSampleId));
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

function sampleTagNames(sample) {
  return (sample.tags || []).map(getTagName);
}

function setSampleTagsEverywhere(sampleID, tags) {
  for (const entry of state.samples) {
    if (getSampleID(entry) === sampleID) {
      entry.tags = tags;
    }
  }
  for (const cluster of state.clusters) {
    for (const member of cluster.members || []) {
      const memberSample = member.sample || member;
      if (getSampleID(memberSample) === sampleID) {
        memberSample.tags = tags;
      }
    }
  }
}

async function saveSampleTags(sample, names) {
  const sampleID = getSampleID(sample);
  if (state.savingTags.has(sampleID)) {
    return false;
  }
  state.savingTags.add(sampleID);

  let saveError = null;
  try {
    const response = await fetch(`/api/samples/${encodeURIComponent(sampleID)}/tags`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ tags: names }),
    });
    if (!response.ok) {
      const message = (await response.text()).trim();
      throw new Error(message || `request failed: ${response.status}`);
    }

    const updated = await response.json();
    const tags = updated.tags || [];
    sample.tags = tags;
    setSampleTagsEverywhere(sampleID, tags);
    state.tags = await loadJSON("/api/tags");
    renderTags();
    if (state.browser === "search") {
      await refreshSamples();
    } else {
      renderClusterSelection();
    }
  } catch (error) {
    saveError = error;
  } finally {
    state.savingTags.delete(sampleID);
  }

  if (saveError) {
    document.getElementById("summary").textContent = `Tag update failed: ${saveError.message}`;
    console.error(saveError);
    if (state.browser === "search") {
      renderSamples();
    } else {
      renderClusterSelection();
    }
    return false;
  }
  return true;
}

function setTagEditorDisabled(tagWrap, disabled) {
  for (const control of tagWrap.querySelectorAll("button, input")) {
    control.disabled = disabled;
  }
}

function createSampleRow(sample, highlighted = false) {
  const li = document.createElement("li");
  li.className = highlighted ? "sample-row active" : "sample-row";

  const meta = document.createElement("div");
  meta.className = "sample-meta";

  const header = document.createElement("div");
  header.className = "sample-header";

  const title = document.createElement("strong");
  title.textContent = getSampleFileName(sample);
  header.appendChild(title);

  const badges = document.createElement("div");
  badges.className = "badge-group";

  const analysis = sample.analysis || sample.Analysis;
  if (analysis) {
    const loopClass = analysis.loop_class ?? analysis.LoopClass;
    if (loopClass) {
      const b = document.createElement("span");
      b.className = `badge-meta badge-${loopClass.toLowerCase()}`;
      b.textContent = loopClass;
      badges.appendChild(b);
    }
    const bpm = analysis.bpm ?? analysis.BPM;
    if (bpm != null) {
      const b = document.createElement("span");
      b.className = "badge-meta badge-bpm";
      b.textContent = `${Math.round(bpm)} BPM`;
      badges.appendChild(b);
    }
    const keyTonic = analysis.key_tonic ?? analysis.KeyTonic;
    if (keyTonic) {
      const keyMode = analysis.key_mode ?? analysis.KeyMode;
      const b = document.createElement("span");
      b.className = "badge-meta badge-key";
      b.textContent = keyMode ? `${keyTonic} ${keyMode}` : keyTonic;
      badges.appendChild(b);
    }
    const chord = analysis.chord ?? analysis.Chord;
    if (chord) {
      const b = document.createElement("span");
      b.className = "badge-meta badge-chord";
      b.textContent = chord;
      badges.appendChild(b);
    }
    const dur = analysis.duration_seconds ?? analysis.DurationSeconds;
    if (dur != null) {
      const b = document.createElement("span");
      b.className = "badge-meta badge-duration";
      b.textContent = `${dur >= 10 ? dur.toFixed(1) : dur.toFixed(2)}s`;
      badges.appendChild(b);
    }
  }

  const sim = sample.similarity ?? sample.Similarity;
  if (sim !== undefined && sim !== null) {
    const b = document.createElement("span");
    b.className = "badge-meta badge-similarity";
    b.textContent = `${Math.round(sim * 100)}% match`;
    badges.appendChild(b);
  }

  header.appendChild(badges);
  meta.appendChild(header);

  const path = document.createElement("div");
  path.className = "sample-path";
  path.textContent = getSamplePath(sample);
  meta.appendChild(path);

  const tagWrap = document.createElement("div");
  tagWrap.className = "tag-cloud sample-tag-editor";
  const tagsSaving = state.savingTags.has(getSampleID(sample));
  for (const tag of sample.tags || []) {
    const tagName = getTagName(tag);
    const chip = document.createElement("span");
    chip.className = "tag tag-editable";

    const label = document.createElement("span");
    label.textContent = tagName;
    chip.appendChild(label);

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "tag-remove";
    remove.disabled = tagsSaving;
    remove.textContent = "×";
    remove.setAttribute("aria-label", `Remove tag ${tagName}`);
    remove.addEventListener("click", async () => {
      setTagEditorDisabled(tagWrap, true);
      const names = sampleTagNames(sample).filter((name) => name !== tagName);
      const saved = await saveSampleTags(sample, names);
      if (!saved) {
        setTagEditorDisabled(tagWrap, false);
      }
    });
    chip.appendChild(remove);
    tagWrap.appendChild(chip);
  }

  const tagInput = document.createElement("input");
  tagInput.className = "tag-input";
  tagInput.type = "text";
  tagInput.placeholder = "Add tag";
  tagInput.disabled = tagsSaving;
  tagInput.setAttribute("aria-label", `Add tag to ${getSampleFileName(sample)}`);
  tagInput.addEventListener("keydown", async (event) => {
    if (event.key !== "Enter") {
      return;
    }
    event.preventDefault();
    const name = tagInput.value.trim();
    if (!name) {
      return;
    }
    setTagEditorDisabled(tagWrap, true);
    const saved = await saveSampleTags(sample, [...sampleTagNames(sample), name]);
    if (!saved) {
      setTagEditorDisabled(tagWrap, false);
    }
  });
  tagWrap.appendChild(tagInput);
  meta.appendChild(tagWrap);

  const player = document.createElement("audio");
  player.controls = true;
  player.preload = "none";
  player.src = `/api/samples/${getSampleID(sample)}/audio`;
  meta.appendChild(player);

  const actions = document.createElement("div");
  actions.className = "sample-actions";

  const findSimilarBtn = document.createElement("button");
  findSimilarBtn.type = "button";
  findSimilarBtn.className = "btn-sm btn-secondary";
  findSimilarBtn.textContent = "Find similar";
  findSimilarBtn.addEventListener("click", async () => {
    state.similarReference = {
      id: getSampleID(sample),
      fileName: getSampleFileName(sample),
    };
    await refreshSamples();
  });
  actions.appendChild(findSimilarBtn);
  meta.appendChild(actions);

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
  const facetType = document.getElementById("facet-type");
  const facetKey = document.getElementById("facet-key");
  const facetChord = document.getElementById("facet-chord");
  const facetBpmMin = document.getElementById("facet-bpm-min");
  const facetBpmMax = document.getElementById("facet-bpm-max");
  const facetDurationMin = document.getElementById("facet-duration-min");
  const facetDurationMax = document.getElementById("facet-duration-max");
  const similarityMetric = document.getElementById("similarity-metric");

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

  similarityMetric.addEventListener("change", async (event) => {
    state.similarityMetric = event.target.value;
    if (state.similarReference) {
      await refreshSamples();
    }
  });

  const onFacetChange = async () => {
    state.facets.type = facetType.value;
    state.facets.key = facetKey.value.trim();
    state.facets.chord = facetChord.value.trim();
    state.facets.bpmMin = facetBpmMin.value.trim();
    state.facets.bpmMax = facetBpmMax.value.trim();
    state.facets.durationMin = facetDurationMin.value.trim();
    state.facets.durationMax = facetDurationMax.value.trim();
    await refreshSamples();
  };

  facetType.addEventListener("change", onFacetChange);
  facetKey.addEventListener("input", onFacetChange);
  facetChord.addEventListener("input", onFacetChange);
  facetBpmMin.addEventListener("input", onFacetChange);
  facetBpmMax.addEventListener("input", onFacetChange);
  facetDurationMin.addEventListener("input", onFacetChange);
  facetDurationMax.addEventListener("input", onFacetChange);

  document.getElementById("clear-filters").addEventListener("click", async () => {
    state.tag = "";
    state.query = "";
    state.similarReference = null;
    state.similarityMetric = "acoustic";
    if (similarityMetric) {
      similarityMetric.value = "acoustic";
    }
    state.facets = {
      type: "",
      bpmMin: "",
      bpmMax: "",
      key: "",
      chord: "",
      durationMin: "",
      durationMax: "",
    };
    search.value = "";
    facetType.value = "";
    facetKey.value = "";
    facetChord.value = "";
    facetBpmMin.value = "";
    facetBpmMax.value = "";
    facetDurationMin.value = "";
    facetDurationMax.value = "";
    await refreshSamples();
    renderTags();
  });

  const [tags, clusters] = await Promise.all([
    loadJSON("/api/tags"),
    loadJSON(`/api/clusters?projection=${encodeURIComponent(state.projection)}`),
  ]);
  state.tags = tags;
  state.clusters = clusters;
  state.activeClusterId = state.clusters[0] ? getClusterID(state.clusters[0]) : null;

  setBrowser("search");
  renderTags();
  renderClusters();
  await refreshSamples();
}

main().catch((error) => {
  console.error(error);
});
