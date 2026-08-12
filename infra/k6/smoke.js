import http from 'k6/http';
import { check, group, sleep } from 'k6';

const webURL = (__ENV.OPENCHAIN_K6_WEB_URL || 'http://localhost:3000').replace(/\/$/, '');
const apiURL = (__ENV.OPENCHAIN_K6_API_URL || 'http://localhost:18091').replace(/\/$/, '');
const requestLimit = Number(__ENV.OPENCHAIN_K6_REQUEST_LIMIT || 8);
const perClientQueueLimit = Number(__ENV.OPENCHAIN_K6_PER_CLIENT_QUEUE_LIMIT || 2);
const expected = http.expectedStatuses(200, 400, 429, 503);
const headers = (ip) => ({ headers: { 'Content-Type': 'application/json', 'Connect-Protocol-Version': '1', 'X-Forwarded-For': ip }, responseCallback: expected });
const address = (suffix) => `0x${String(suffix).padStart(40, '0')}`;
const post = (path, body, ip) => http.post(`${apiURL}${path}`, JSON.stringify(body), headers(ip));
const trace = (seedAddress, ip) => post('/openchain.v1.TracingService/TraceGraph', { seedAddress, network: 'NETWORK_ETHEREUM_MAINNET', limit: 1 }, ip);
const traceRequest = (seedAddress, ip) => ({ method: 'POST', url: `${apiURL}/openchain.v1.TracingService/TraceGraph`, body: JSON.stringify({ seedAddress, network: 'NETWORK_ETHEREUM_MAINNET', limit: 1 }), params: headers(ip) });

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: { checks: ['rate==1'], http_req_duration: ['p(95)<2000'] },
};

export default function () {
  group('UI and API latency', () => {
    const web = http.get(webURL, { responseCallback: expected });
    check(web, { 'web returns OpenChain': (response) => response.status === 200 && response.body.includes('<title>OpenChain') });
    const health = http.get(`${apiURL}/api/v1/health`, { responseCallback: expected });
    check(health, { 'stub API is healthy': (response) => response.status === 200 && response.body.includes('"service":"openchain-api"') });
    const lookup = post('/openchain.v1.LookupService/LookupAddress', { address: address(1), network: 'NETWORK_ETHEREUM_MAINNET' }, '198.18.0.1');
    check(lookup, { 'lookup succeeds': (response) => response.status === 200 && response.body.includes('"summary"') });
  });

  group('shared IP rate limit', () => {
    const requests = Array.from({ length: requestLimit + 1 }, () => ({ method: 'POST', url: `${apiURL}/openchain.v1.LookupService/LookupAddress`, body: JSON.stringify({ address: 'invalid', network: 'NETWORK_ETHEREUM_MAINNET' }), params: headers('198.18.0.2') }));
    const responses = http.batch(requests);
    check(responses, { 'shared IP is capped': (items) => items.filter((item) => item.status === 400).length === requestLimit && items.filter((item) => item.status === 429).length === 1 });
  });

  const queueIP = '198.18.0.3';
  group('queue fairness and saturation', () => {
    const clientRequests = Array.from({ length: perClientQueueLimit + 1 }, (_, index) => traceRequest(address(100 + index), queueIP));
    const clientResponses = http.batch(clientRequests);
    check(clientResponses, { 'one client cannot fill its queue share': (items) => items.filter((item) => item.status === 200).length === perClientQueueLimit && items.filter((item) => item.status === 429).length === 1 });
    const secondClient = http.batch([traceRequest(address(200), '198.18.0.4'), traceRequest(address(201), '198.18.0.4')]);
    check(secondClient, { 'another client gets its queue share': (items) => items.every((item) => item.status === 200) });
    const saturated = trace(address(300), '198.18.0.5');
    check(saturated, { 'global queue reports saturation': (response) => response.status === 429 });
  });

  group('status polling stays free', () => {
    sleep(2);
    const polls = http.batch(Array.from({ length: requestLimit + 2 }, () => ({ method: 'POST', url: `${apiURL}/openchain.v1.TracingService/GetTraceStatus`, body: JSON.stringify({ address: address(100), network: 'NETWORK_ETHEREUM_MAINNET', limit: 1 }), params: headers(queueIP) })));
    check(polls, {
      'status polling is never rate limited': (items) => items.every((item) => item.status === 200),
      'a controlled provider trace completed': (items) => items.some((item) => item.body.includes('load-test-stub')),
    });
  });
}
