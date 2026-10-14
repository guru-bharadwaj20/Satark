// Satark load-test suite.
//
//   k6 run -e BASE_URL=https://api.satark.example -e PROFILE=load tests/load/scenarios.js
//
// PROFILE: smoke | load | stress | spike
// Traffic mix mirrors production: ~80% text checks, ~20% link checks.
import { sleep } from 'k6';
import { checkText, checkURL } from './lib/client.js';
import { texts, urls, pick } from './lib/payloads.js';

const PROFILE = __ENV.PROFILE || 'smoke';

const profiles = {
  smoke: {
    executor: 'constant-vus',
    vus: 2,
    duration: '1m',
  },
  load: {
    executor: 'ramping-arrival-rate',
    startRate: 5,
    timeUnit: '1s',
    preAllocatedVUs: 50,
    maxVUs: 300,
    stages: [
      { target: 25, duration: '3m' },
      { target: 50, duration: '10m' },
      { target: 0, duration: '2m' },
    ],
  },
  stress: {
    executor: 'ramping-arrival-rate',
    startRate: 10,
    timeUnit: '1s',
    preAllocatedVUs: 100,
    maxVUs: 1000,
    // Keep stepping up until something breaks; the first threshold to fail
    // marks the bottleneck.
    stages: [
      { target: 50, duration: '3m' },
      { target: 100, duration: '3m' },
      { target: 200, duration: '3m' },
      { target: 400, duration: '3m' },
      { target: 0, duration: '2m' },
    ],
  },
  spike: {
    executor: 'ramping-arrival-rate',
    startRate: 5,
    timeUnit: '1s',
    preAllocatedVUs: 100,
    maxVUs: 800,
    // A message forwarded to a large WhatsApp group: sudden 20x burst.
    stages: [
      { target: 10, duration: '2m' },
      { target: 200, duration: '20s' },
      { target: 200, duration: '2m' },
      { target: 10, duration: '20s' },
      { target: 10, duration: '3m' },
    ],
  },
};

export const options = {
  scenarios: { [PROFILE]: profiles[PROFILE] },
  thresholds: {
    'http_req_failed': ['rate<0.01'],
    'http_req_duration{name:check_text}': ['p(95)<500', 'p(99)<1500'],
    'http_req_duration{name:check_url}': ['p(95)<3000'],
    'checks': ['rate>0.99'],
  },
  summaryTrendStats: ['avg', 'p(50)', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

export default function () {
  if (Math.random() < 0.8) {
    checkText(pick(texts, __ITER));
  } else {
    checkURL(pick(urls, __ITER));
  }
  if (PROFILE === 'smoke') sleep(1);
}
