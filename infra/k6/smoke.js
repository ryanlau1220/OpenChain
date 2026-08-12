import http from 'k6/http';
import { check } from 'k6';

const baseURL = (__ENV.OPENCHAIN_K6_BASE_URL || 'http://localhost:3000').replace(/\/$/, '');
const vus = Number(__ENV.OPENCHAIN_K6_VUS || 2);
const iterations = Number(__ENV.OPENCHAIN_K6_ITERATIONS || 4);
const healthStatus = http.expectedStatuses(200, 503);

export const options = {
  vus,
  iterations,
  thresholds: {
    checks: ['rate==1'],
    http_req_duration: ['p(95)<2000'],
  },
};

export default function () {
  const web = http.get(baseURL);
  check(web, { 'web returns OpenChain': (response) => response.status === 200 && response.body.includes('<title>OpenChain') });

  const health = http.get(`${baseURL}/api/v1/health`, { responseCallback: healthStatus });
  check(health, { 'health is observable': (response) => (response.status === 200 || response.status === 503) && response.body.includes('"service":"openchain-api"') });
}
