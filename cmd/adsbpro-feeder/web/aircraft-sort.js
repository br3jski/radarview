(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  root.ADSBProAircraftSort = api;
}(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  const text = value => {
    const normalized = String(value || '').trim().toUpperCase();
    return normalized || null;
  };
  const number = value => {
    const normalized = Number(value);
    return value == null || !Number.isFinite(normalized) ? null : normalized;
  };
  const timestamp = value => {
    const normalized = Date.parse(value);
    return value == null || !Number.isFinite(normalized) ? null : normalized;
  };

  const values = {
    icao: row => text(row.icao),
    callsign: row => text(row.callsign),
    altitude: row => row.onGround ? -1 : number(row.altitudeFeet),
    speed: row => number(row.speedKnots),
    track: row => number(row.trackDegrees),
    distance: row => number(row.distanceNm),
    rssi: row => number(row.rssiDbfs),
    localRate: row => number(row.localMessagesPerSecond),
    sentRate: row => number(row.sentMessagesPerSecond),
    lastSent: row => timestamp(row.lastSentAt),
    lastSeen: row => timestamp(row.lastSeenAt),
    reason: row => text(row.reason),
  };

  const ascendingByDefault = new Set(['icao', 'callsign', 'reason']);

  function sortRows(rows, key, direction) {
    const value = values[key];
    if (!value) return rows.slice();
    const multiplier = direction === 'asc' ? 1 : -1;
    return rows
      .map((row, index) => ({ row, index, value: value(row) }))
      .sort((left, right) => {
        if (left.value == null && right.value == null) return left.index - right.index;
        if (left.value == null) return 1;
        if (right.value == null) return -1;
        const comparison = typeof left.value === 'string'
          ? left.value.localeCompare(right.value)
          : left.value - right.value;
        if (comparison !== 0) return comparison * multiplier;
        const byICAO = String(left.row.icao || '').localeCompare(String(right.row.icao || ''));
        return byICAO || left.index - right.index;
      })
      .map(entry => entry.row);
  }

  function nextSort(current, key) {
    if (current.key === key) {
      return { key, direction: current.direction === 'asc' ? 'desc' : 'asc' };
    }
    return { key, direction: ascendingByDefault.has(key) ? 'asc' : 'desc' };
  }

  return { nextSort, sortRows };
}));
