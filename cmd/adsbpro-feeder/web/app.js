const byId = id => document.getElementById(id);
let statusData = null;
let selectedPlatform = 'linux';
const aircraftSortState = {
  sent: { key: 'sentRate', direction: 'desc' },
  notSent: { key: 'lastSeen', direction: 'desc' },
};

const formatRate = value => {
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s'];
  let number = Number(value) || 0;
  let index = 0;
  while (number >= 1000 && index < units.length - 1) {
    number /= 1000;
    index++;
  }
  return { number: number.toFixed(index === 0 ? 0 : number < 10 ? 1 : 0), unit: units[index] };
};

const ago = value => {
  if (!value) return '—';
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 2) return 'now';
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  return `${Math.floor(seconds / 3600)}h ago`;
};

const shortId = value => value && value.length > 13 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value || '—';
const dateTime = value => value ? new Date(value).toLocaleString() : '—';
const number = (value, digits = 0) => value == null ? '—' : Number(value).toFixed(digits);
const altitude = row => row.onGround ? 'GROUND' : row.altitudeFeet == null ? '—' : `${number(row.altitudeFeet)} ft`;
const speed = row => row.speedKnots == null ? '—' : `${number(row.speedKnots)} kt`;
const track = row => row.trackDegrees == null ? '—' : `${number(row.trackDegrees)}°`;
const distance = row => row.distanceNm == null ? '—' : `${number(row.distanceNm, 1)} NM`;
const rssi = row => row.rssiDbfs == null ? '—' : `${number(row.rssiDbfs, 1)} dBFS`;
const messageRate = value => number(value || 0, 1);

function cell(value, className = '') {
  const element = document.createElement('td');
  element.textContent = value;
  if (className) element.className = className;
  element.title = value;
  return element;
}

function updateSortHeaders(tableName) {
  document.querySelectorAll(`[data-sort-table="${tableName}"]`).forEach(header => {
    const active = header.dataset.sortKey === aircraftSortState[tableName].key;
    header.setAttribute('aria-sort', active
      ? aircraftSortState[tableName].direction === 'asc' ? 'ascending' : 'descending'
      : 'none');
  });
}

function renderAircraftRows(targetId, rows, tableName, notSent, unavailable) {
  const body = byId(targetId);
  body.replaceChildren();
  updateSortHeaders(tableName);
  if (!rows.length) {
    const row = document.createElement('tr');
    const message = unavailable
      ? 'aircraft.json is unavailable — feeding continues normally'
      : notSent ? 'All recently seen aircraft are being sent' : 'No aircraft sent during the last 30 seconds';
    const empty = cell(message, 'empty');
    empty.colSpan = notSent ? 11 : 10;
    row.append(empty);
    body.append(row);
    return;
  }
  const sortedRows = ADSBProAircraftSort.sortRows(
    rows,
    aircraftSortState[tableName].key,
    aircraftSortState[tableName].direction,
  );
  for (const aircraft of sortedRows) {
    const row = document.createElement('tr');
    row.append(
      cell(aircraft.icao || '—', 'icao'),
      cell(aircraft.callsign || '—'),
      cell(altitude(aircraft)),
      cell(speed(aircraft)),
      cell(track(aircraft)),
      cell(distance(aircraft)),
      cell(rssi(aircraft)),
      cell(messageRate(aircraft.localMessagesPerSecond)),
      cell(messageRate(aircraft.sentMessagesPerSecond)),
      cell(ago(notSent ? aircraft.lastSeenAt : aircraft.lastSentAt)),
    );
    if (notSent) row.append(cell(aircraft.reason || 'Not sent', 'reason'));
    body.append(row);
  }
}

function renderAircraft(data) {
  const traffic = data.aircraftTraffic || { metadataAvailable: false, sent: [], notSent: [] };
  const sent = Array.isArray(traffic.sent) ? traffic.sent : [];
  const notSent = Array.isArray(traffic.notSent) ? traffic.notSent : [];
  byId('sent-count').textContent = sent.length;
  byId('not-sent-count').textContent = notSent.length;
  byId('aircraft-source-note').textContent = traffic.metadataAvailable
    ? `Decoder data: ${traffic.source || 'aircraft.json'}`
    : 'aircraft.json unavailable — sent aircraft can still be detected from the outgoing stream';
  renderAircraftRows('sent-aircraft', sent, 'sent', false, false);
  renderAircraftRows('not-sent-aircraft', notSent, 'notSent', true, !traffic.metadataAvailable);
}

function render(data) {
  statusData = data;
  const uploadRate = formatRate(data.traffic.payloadBytesPerSecond);
  byId('state').textContent = (data.state || 'unknown').toUpperCase();
  byId('state-dot').className = `dot ${data.state || ''}`;
  byId('aircraft').textContent = data.source.aircraft ?? '—';
  byId('aircraft-note').textContent = data.source.aircraft == null ? 'Not reported by local decoder' : 'Seen during the last 30 seconds';
  byId('rate').textContent = Number(data.traffic.messagesPerSecond || 0).toFixed(1);
  byId('last-frame').textContent = ago(data.source.lastFrameAt);
  byId('source').textContent = `${(data.source.format || data.source.mode).toUpperCase()} · ${data.source.host}:${data.source.port}`;
  byId('upload-rate').textContent = uploadRate.number;
  byId('upload-unit').textContent = uploadRate.unit;
  byId('account').textContent = data.accountDisplay || 'Not supplied';
  byId('installation').textContent = shortId(data.installationId);
  byId('installation').title = data.installationId || '';
  byId('connected').textContent = dateTime(data.connectedAt);
  byId('version').textContent = data.version || '—';
  byId('error').textContent = data.error || '';
  byId('error').classList.toggle('hidden', !data.error);
  byId('update-button').classList.toggle('hidden', !data.update.available);
  byId('latest-version').textContent = data.update.latestVersion || '';
  renderAircraft(data);
  renderCommand();
}

function renderCommand() {
  if (!statusData) return;
  byId('update-command').textContent = selectedPlatform === 'linux' ? statusData.update.linuxCommand : statusData.update.windowsCommand;
  byId('linux-tab').classList.toggle('selected', selectedPlatform === 'linux');
  byId('windows-tab').classList.toggle('selected', selectedPlatform === 'windows');
}

async function refresh() {
  try {
    const response = await fetch('/api/status', { cache: 'no-store' });
    if (response.ok) render(await response.json());
  } catch {
    byId('state').textContent = 'STATUS UNAVAILABLE';
    byId('state-dot').className = 'dot disconnected';
  }
}

byId('update-button').addEventListener('click', () => byId('update-dialog').showModal());
byId('linux-tab').addEventListener('click', () => { selectedPlatform = 'linux'; renderCommand(); });
byId('windows-tab').addEventListener('click', () => { selectedPlatform = 'windows'; renderCommand(); });
document.querySelectorAll('[data-sort-table]').forEach(header => {
  const button = header.querySelector('button');
  button.addEventListener('click', () => {
    const tableName = header.dataset.sortTable;
    aircraftSortState[tableName] = ADSBProAircraftSort.nextSort(
      aircraftSortState[tableName],
      header.dataset.sortKey,
    );
    if (statusData) renderAircraft(statusData);
    else updateSortHeaders(tableName);
  });
});
byId('copy-command').addEventListener('click', async () => {
  const command = byId('update-command').textContent;
  try {
    await navigator.clipboard.writeText(command);
    byId('copy-command').textContent = 'COPIED';
  } catch {
    const range = document.createRange();
    range.selectNodeContents(byId('update-command'));
    getSelection().removeAllRanges();
    getSelection().addRange(range);
    byId('copy-command').textContent = 'SELECTED';
  }
  setTimeout(() => byId('copy-command').textContent = 'COPY COMMAND', 1500);
});

refresh();
setInterval(refresh, 2000);
