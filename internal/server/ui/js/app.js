// hsdebug UI — talks to the same REST API the CLI mirrors.

function setStatus(msg, isError) {
  const el = document.getElementById('status');
  el.textContent = msg || '';
  el.style.color = isError ? 'var(--token-color-palette-red-200)' : '';
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
    list.innerHTML = services
      .map((s) => {
        const h = byName[s.name] || {};
        const cls = h.healthy ? 'healthy' : 'unhealthy';
        return `<div class="card">
          <span class="dot ${cls}"></span>
          <div><strong>${s.name}</strong>
          <div class="muted">${s.scheme}://${s.host}:${s.port}${s.healthPath || ''}</div></div>
        </div>`;
      })
      .join('');
  }

  const a = agentRes || {};
  document.getElementById('agent').textContent =
    `agent: ${a.model || 'unset'} — ${a.configured ? 'ready' : a.detail || 'not configured'}`;
}

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
  document.getElementById('btn-register').addEventListener('click', () => showRegisterForm(true));
  document.getElementById('btn-register-2').addEventListener('click', () => showRegisterForm(true));
  document.getElementById('btn-scan').addEventListener('click', scan);
  document.getElementById('btn-scan-2').addEventListener('click', scan);
  document.getElementById('reg-submit').addEventListener('click', submitRegister);
  document.getElementById('reg-cancel').addEventListener('click', () => showRegisterForm(false));
}

wire();
load();
