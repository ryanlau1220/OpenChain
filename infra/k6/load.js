import http from 'k6/http';
import { check, group } from 'k6';

const webURL = (__ENV.OPENCHAIN_K6_WEB_URL || 'http://localhost:3000').replace(/\/$/, '');
const apiURL = (__ENV.OPENCHAIN_K6_API_URL || 'http://localhost:18091').replace(/\/$/, '');
const vus = Number(__ENV.OPENCHAIN_K6_LOAD_VUS || 5);
const duration = __ENV.OPENCHAIN_K6_LOAD_DURATION || '10s';
const expected = http.expectedStatuses(200, 404);
const headers = (ip) => ({ headers: { 'Content-Type': 'application/json', 'Connect-Protocol-Version': '1', 'X-Forwarded-For': ip }, responseCallback: expected });
const address = (suffix) => `0x${String(suffix).padStart(40, '0')}`;

function testClientIP() {
  const sequence = (__VU - 1) * 100000 + __ITER;
  const second = 18 + Math.floor(sequence / (254 * 254));
  const third = Math.floor(sequence / 254) % 254 + 1;
  const fourth = sequence % 254 + 1;
  return `198.${second}.${third}.${fourth}`;
}

export const options = {
  scenarios: { controlled_routes: { executor: 'constant-vus', vus, duration } },
  thresholds: {
    checks: ['rate>0.99'],
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<1000'],
  },
};

export default function () {
  const ip = testClientIP();
  group('controlled UI and read API load', () => {
    const responses = http.batch([
      ['GET', webURL, null, { responseCallback: expected }],
      ['POST', `${apiURL}/openchain.v1.LookupService/LookupAddress`, JSON.stringify({ address: address(__VU), network: 'NETWORK_ETHEREUM_MAINNET' }), headers(ip)],
      ['POST', `${apiURL}/openchain.v1.TracingService/GetTraceStatus`, JSON.stringify({ address: address(100), network: 'NETWORK_ETHEREUM_MAINNET', limit: 1 }), headers(ip)],
    ]);
    check(responses, {
      'UI route is available': (items) => items[0].status === 200 && items[0].body.includes('<title>OpenChain'),
      'lookup remains available': (items) => items[1].status === 200,
      'status polling is never throttled': (items) => items[2].status === 200 || items[2].status === 404,
    });
  });
}
