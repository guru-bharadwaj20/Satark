// Constant-rate soak used while running playbooks/upgrade.yml.
// Every failed request is counted so the upgrade can be graded on error count.
//
//   k6 run -e BASE_URL=https://api.satark.example tests/load/upgrade-soak.js
import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const RATE = parseInt(__ENV.RATE || '20', 10);
const DURATION = __ENV.DURATION || '90m';

export const upgradeErrors = new Counter('satark_upgrade_errors');

export const options = {
  scenarios: {
    soak: {
      executor: 'constant-arrival-rate',
      rate: RATE,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: RATE * 2,
      maxVUs: RATE * 10,
    },
  },
  thresholds: {
    // An upgrade should be invisible to users: allow at most 0.1% failures.
    http_req_failed: ['rate<0.001'],
    http_req_duration: ['p(95)<800'],
  },
  summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'p(99)', 'max'],
};

const messages = [
  'Your KYC is pending, update now at http://example.test/kyc to avoid account block',
  'Hi, are we still meeting at 6 today?',
  'Congratulations! You won Rs 25,00,000 in the lucky draw. Pay Rs 999 processing fee.',
];

export default function () {
  const body = JSON.stringify({ text: messages[__ITER % messages.length], lang: 'en' });
  const res = http.post(`${BASE_URL}/v1/check/text`, body, {
    headers: { 'Content-Type': 'application/json' },
    timeout: '10s',
    tags: { name: 'check_text' },
  });
  const ok = check(res, { 'status is 200': (r) => r.status === 200 });
  if (!ok) {
    upgradeErrors.add(1, { status: String(res.status) });
  }
}
