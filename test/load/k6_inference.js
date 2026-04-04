import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const ttft = new Trend('time_to_first_token', true);

// Test configuration
export const options = {
  scenarios: {
    // Ramp up load test
    ramp_up: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '30s', target: 10 },
        { duration: '1m', target: 50 },
        { duration: '2m', target: 100 },
        { duration: '30s', target: 0 },
      ],
    },
    // Constant load for SLA verification
    sla_check: {
      executor: 'constant-arrival-rate',
      rate: 100,
      timeUnit: '1m',
      duration: '5m',
      preAllocatedVUs: 20,
      startTime: '5m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<5000', 'p(99)<10000'],
    errors: ['rate<0.01'],
    time_to_first_token: ['p(95)<2000'],
  },
};

const BASE_URL = __ENV.TAAS_URL || 'http://localhost:8080';
const API_KEY = __ENV.TAAS_API_KEY || 'test-key';

export function setup() {
  // Health check
  const res = http.get(`${BASE_URL}/health`);
  check(res, { 'health ok': (r) => r.status === 200 });
  return { apiKey: API_KEY };
}

export default function (data) {
  const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${data.apiKey}`,
  };

  const payload = JSON.stringify({
    model: 'llama-3-8b',
    messages: [
      { role: 'user', content: 'Hello, how are you?' },
    ],
    max_tokens: 50,
    temperature: 0.7,
  });

  const start = Date.now();
  const res = http.post(`${BASE_URL}/v1/chat/completions`, payload, {
    headers,
    timeout: '30s',
  });

  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'has choices': (r) => {
      try {
        return JSON.parse(r.body).choices !== undefined;
      } catch {
        return false;
      }
    },
    'has usage': (r) => {
      try {
        return JSON.parse(r.body).usage !== undefined;
      } catch {
        return false;
      }
    },
  });

  errorRate.add(!success);

  // Approximate TTFT (for non-streaming, use response time)
  ttft.add(Date.now() - start);

  sleep(Math.random() * 2 + 0.5);
}

export function teardown(data) {
  console.log('Load test complete');
}
