async function loadJSON(path) {
  const response = await fetch(path);
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json();
}

function renderList(id, items, render) {
  const root = document.getElementById(id);
  root.innerHTML = "";
  for (const item of items) {
    const li = document.createElement("li");
    li.textContent = render(item);
    root.appendChild(li);
  }
}

async function main() {
  const [samples, tags] = await Promise.all([
    loadJSON("/api/samples"),
    loadJSON("/api/tags"),
  ]);

  renderList("samples", samples, (sample) => {
    const tagNames = (sample.tags || []).map((tag) => tag.NormalizedName).join(", ");
    return tagNames ? `${sample.FileName} — ${tagNames}` : sample.FileName;
  });

  renderList("tags", tags, (entry) => `${entry.Tag.NormalizedName} (${entry.SampleCount})`);
}

main().catch((error) => {
  console.error(error);
});
