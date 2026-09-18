// hsdebug UI — talks to the same REST API the CLI mirrors.

// ----- Theme -----
function initTheme() {
  let theme = localStorage.getItem('hsdebug-theme');
  if (!theme) {
    theme = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  applyTheme(theme);
}
function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem('hsdebug-theme', theme);
  const btn = document.getElementById('theme-toggle');
  if (btn) btn.innerHTML = theme === 'dark' ? '\u2600' : '\u263D'; // sun / moon
}
function toggleTheme() {
  const cur = document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';
  applyTheme(cur === 'dark' ? 'light' : 'dark');
}

// ----- State -----
const selected = new Set();
let godMode = false;

function setStatus(msg, isError) {
  const el = document.getElementById('status');
  el.textContent = msg || '';
  el.style.color = isError ? 'var(--bad)' : '';
}

async function load() {
  setStatus('');
  let svcRes, agentRes;
  try {
    [svcRes, agentRes] = await Promise.all([
      fetch('/api/services').then((r) => r.json()),
      fetch('/api/agent').then((r) => r.json()),
    ]);
  } catch (e) {
    setStatus('failed to reach hsdebug API: ' + e, true);
    return;
  }

  const services = svcRes.services || [];
  const firstRun = document.getElementById('first-run');
  const toolbar = document.getElementById('toolbar');
  const list = document.getElementById('services');

  if (services.length === 0) {
    firstRun.style.display = 'flex';
    toolbar.style.display = 'none';
    list.innerHTML = '';
    updateSelectionBar([]);
  } else {
    firstRun.style.display = 'none';
    toolbar.style.display = 'flex';
    let byName = {};
    try {
      const health = await fetch('/api/health').then((r) => r.json());
      (health.results || []).forEach((h) => (byName[h.name] = h));
    } catch (e) {
      /* health optional */
    }
    // Drop selections for services that no longer exist.
    const names = services.map((s) => s.name);
    [...selected].forEach((n) => { if (!names.includes(n)) selected.delete(n); });

    list.innerHTML = services
      .map((s) => {
        const h = byName[s.name] || {};
        const cls = h.healthy ? 'healthy' : 'unhealthy';
        const sel = selected.has(s.name) ? ' selected' : '';
        return `<div class="card selectable${sel}" data-name="${s.name}">
          <span class="check">${selected.has(s.name) ? '\u2713' : ''}</span>
          <span class="dot ${cls}"></span>
          <div><strong>${s.name}</strong>
          <div class="muted">${s.scheme}://${s.host}:${s.port}${s.healthPath || ''}</div></div>
        </div>`;
      })
      .join('');

    list.querySelectorAll('.card.selectable').forEach((card) => {
      card.addEventListener('click', () => toggleSelect(card.dataset.name));
    });
    updateSelectionBar(names);
  }

  const a = agentRes || {};
  godMode = !!a.godMode;
  updateGodModeButton();
  document.getElementById('agent').textContent =
    `agent: ${a.model || 'unset'} — ${a.configured ? 'ready' : a.detail || 'not configured'}`;
}

function toggleSelect(name) {
  if (selected.has(name)) selected.delete(name);
  else selected.add(name);
  load();
}

function updateSelectionBar(allNames) {
  const bar = document.getElementById('selection-bar');
  const count = document.getElementById('selection-count');
  if (allNames.length === 0) {
    bar.style.display = 'none';
    return;
  }
  bar.style.display = 'flex';
  count.textContent = `${selected.size} selected`;
  document.getElementById('btn-debug').disabled = selected.size === 0;
  bar._allNames = allNames;
}

// ----- God mode -----
function updateGodModeButton() {
  const btn = document.getElementById('godmode-toggle');
  if (!btn) return;
  btn.textContent = 'GOD MODE: ' + (godMode ? 'ON' : 'OFF');
  btn.classList.toggle('on', godMode);
}
async function toggleGodMode() {
  const next = !godMode;
  try {
    const res = await fetch('/api/agent/godmode', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: next }),
    });
    const data = await res.json();
    if (!res.ok) { setStatus('god mode: ' + (data.error || res.status), true); return; }
    godMode = !!data.godMode;
    updateGodModeButton();
    setStatus(godMode ? 'God mode ON — opencode runs with elevated permissions.' : 'God mode off.');
  } catch (e) {
    setStatus('god mode failed: ' + e, true);
  }
}

// ----- Debug + streaming -----
async function debugSelected() {
  const names = [...selected];
  if (names.length === 0) return;
  setStatus(`Starting debug for ${names.length} service(s)…`);
  let sess;
  try {
    const res = await fetch('/api/debug', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ services: names, auto: godMode }),
    });
    sess = await res.json();
    if (!res.ok) { setStatus('debug failed: ' + (sess.error || res.status), true); return; }
  } catch (e) {
    setStatus('debug failed: ' + e, true);
    return;
  }
  openSession(sess.id, names);
}

function openSession(id, names) {
  const container = document.getElementById('session-container');
  container.innerHTML = `<div class="session">
    <div class="session-head">Debug session <span class="muted">${names.join(', ')}</span>
      ${godMode ? '<span class="badge">GOD MODE</span>' : ''}</div>
    <div class="perm-container" id="perm-${id}"></div>
    <div class="session-log" id="log-${id}"></div>
  </div>`;
  const log = document.getElementById('log-' + id);
  const es = new EventSource('/api/debug/stream?id=' + encodeURIComponent(id));
  es.addEventListener('line', (ev) => {
    log.textContent += ev.data + '\n';
    log.scrollTop = log.scrollHeight;
  });
  es.addEventListener('permission', (ev) => {
    renderPermission(id, JSON.parse(ev.data));
  });
  es.addEventListener('done', (ev) => {
    log.textContent += '\n[session ' + (ev.data || 'done') + ']\n';
    es.close();
    load();
  });
  es.onerror = () => { es.close(); };
}

function renderPermission(id, req) {
  const holder = document.getElementById('perm-' + id);
  if (!holder) return;
  const reqId = req.requestId || req.id || '';
  holder.innerHTML = `<div class="perm-prompt">
    <div><strong>opencode requests permission:</strong> ${req.title || req.type || 'action'}</div>
    <div class="muted">${(req.detail || '')}</div>
    <div class="perm-actions">
      <button class="btn" data-d="allow">Approve</button>
      <button class="btn secondary" data-d="deny">Deny</button>
    </div>
  </div>`;
  holder.querySelectorAll('button').forEach((b) => {
    b.addEventListener('click', async () => {
      await fetch('/api/debug/permission', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id, requestId: reqId, decision: b.dataset.d }),
      });
      holder.innerHTML = '';
    });
  });
}

// ----- Scan / register (unchanged behavior) -----
async function scan() {
  setStatus('Scanning loopback for services…');
  try {
    const res = await fetch('/api/scan', { method: 'POST' });
    const data = await res.json();
    if (!res.ok) {
      setStatus('scan failed: ' + (data.error || res.status), true);
      return;
    }
    setStatus(`Scan complete: ${data.added || 0} newly registered, ${(data.detected || []).length} detected.`);
  } catch (e) {
    setStatus('scan failed: ' + e, true);
    return;
  }
  load();
}

function showRegisterForm(show) {
  document.getElementById('register-form').style.display = show ? 'flex' : 'none';
}

async function submitRegister() {
  const name = document.getElementById('reg-name').value.trim();
  const port = parseInt(document.getElementById('reg-port').value, 10);
  const catalogId = document.getElementById('reg-catalog').value.trim();
  const healthPath = document.getElementById('reg-health').value.trim();
  if (!name || !port) {
    setStatus('name and port are required', true);
    return;
  }
  try {
    const res = await fetch('/api/services', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, port, catalogId, healthPath }),
    });
    const data = await res.json();
    if (!res.ok) {
      setStatus('register failed: ' + (data.error || res.status), true);
      return;
    }
    setStatus(`Registered ${name}.`);
    showRegisterForm(false);
    document.getElementById('reg-name').value = '';
    document.getElementById('reg-port').value = '';
    document.getElementById('reg-catalog').value = '';
    document.getElementById('reg-health').value = '';
  } catch (e) {
    setStatus('register failed: ' + e, true);
    return;
  }
  load();
}

function wire() {
  document.getElementById('theme-toggle').addEventListener('click', toggleTheme);
  document.getElementById('godmode-toggle').addEventListener('click', toggleGodMode);
  document.getElementById('btn-register').addEventListener('click', () => showRegisterForm(true));
  document.getElementById('btn-register-2').addEventListener('click', () => showRegisterForm(true));
  document.getElementById('btn-scan').addEventListener('click', scan);
  document.getElementById('btn-scan-2').addEventListener('click', scan);
  document.getElementById('reg-submit').addEventListener('click', submitRegister);
  document.getElementById('reg-cancel').addEventListener('click', () => showRegisterForm(false));
  document.getElementById('btn-debug').addEventListener('click', debugSelected);
  document.getElementById('btn-clear').addEventListener('click', () => { selected.clear(); load(); });
  document.getElementById('btn-select-all').addEventListener('click', () => {
    const bar = document.getElementById('selection-bar');
    (bar._allNames || []).forEach((n) => selected.add(n));
    load();
  });
}

initTheme();
wire();
load();
