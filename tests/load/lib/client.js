import http from 'k6/http';
import { check } from 'k6';

export const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export function scanText(payload) {
  const res = http.post(`${BASE_URL}/v1/scan/text`, JSON.stringify(payload), {
    headers: { 'Content-Type': 'application/json' },
    timeout: '10s',
    tags: { name: 'scan_text' },
  });
  check(res, {
    'text: 200': (r) => r.status === 200,
    'text: has verdict': (r) => r.status !== 200 || r.json('verdict') !== undefined,
  });
  return res;
}

// image is an ArrayBuffer from open(path, 'b') in the init context.
export function scanQR(image, name) {
  const res = http.post(
    `${BASE_URL}/v1/scan/qr`,
    { image: http.file(image, name, 'image/png'), lang: 'en' },
    { timeout: '10s', tags: { name: 'scan_qr' } },
  );
  check(res, {
    'qr: 200': (r) => r.status === 200,
    'qr: has verdict': (r) => r.status !== 200 || r.json('verdict') !== undefined,
  });
  return res;
}
