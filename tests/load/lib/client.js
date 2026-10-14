import http from 'k6/http';
import { check } from 'k6';

export const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const API_KEY = __ENV.API_KEY || '';

function headers() {
  const h = { 'Content-Type': 'application/json' };
  if (API_KEY) h['X-API-Key'] = API_KEY;
  return h;
}

export function checkText(payload) {
  const res = http.post(`${BASE_URL}/v1/check/text`, JSON.stringify(payload), {
    headers: headers(),
    timeout: '10s',
    tags: { name: 'check_text' },
  });
  check(res, {
    'text: 200': (r) => r.status === 200,
    'text: has verdict': (r) => r.status !== 200 || r.json('verdict') !== undefined,
  });
  return res;
}

export function checkURL(url) {
  const res = http.post(`${BASE_URL}/v1/check/url`, JSON.stringify({ url }), {
    headers: headers(),
    timeout: '30s',
    tags: { name: 'check_url' },
  });
  check(res, { 'url: 200 or 202': (r) => r.status === 200 || r.status === 202 });
  return res;
}
