'use strict';

const assert = require('node:assert/strict');
const { nextSort, sortRows } = require('../cmd/adsbpro-feeder/web/aircraft-sort.js');

const rows = [
  {
    icao: 'BBBBBB', callsign: 'Zulu', altitudeFeet: 2000, distanceNm: null,
    sentMessagesPerSecond: 2, lastSentAt: '2026-07-29T12:00:00Z',
  },
  {
    icao: 'AAAAAA', callsign: 'Alpha', altitudeFeet: 1000, distanceNm: 20,
    sentMessagesPerSecond: 5, lastSentAt: '2026-07-29T12:00:02Z',
  },
  {
    icao: 'CCCCCC', callsign: '', onGround: true, distanceNm: 10,
    sentMessagesPerSecond: 1, lastSentAt: '2026-07-29T12:00:01Z',
  },
];

assert.deepEqual(
  sortRows(rows, 'sentRate', 'desc').map(row => row.icao),
  ['AAAAAA', 'BBBBBB', 'CCCCCC'],
);
assert.deepEqual(
  sortRows(rows, 'icao', 'asc').map(row => row.icao),
  ['AAAAAA', 'BBBBBB', 'CCCCCC'],
);
assert.deepEqual(
  sortRows(rows, 'distance', 'asc').map(row => row.icao),
  ['CCCCCC', 'AAAAAA', 'BBBBBB'],
  'missing values must remain last',
);
assert.deepEqual(
  sortRows(rows, 'lastSent', 'desc').map(row => row.icao),
  ['AAAAAA', 'CCCCCC', 'BBBBBB'],
);
assert.deepEqual(nextSort({ key: 'sentRate', direction: 'desc' }, 'sentRate'), {
  key: 'sentRate', direction: 'asc',
});
assert.deepEqual(nextSort({ key: 'sentRate', direction: 'desc' }, 'callsign'), {
  key: 'callsign', direction: 'asc',
});
assert.deepEqual(nextSort({ key: 'icao', direction: 'asc' }, 'distance'), {
  key: 'distance', direction: 'desc',
});

console.log('Aircraft sorting tests passed.');
