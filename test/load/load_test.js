/**
 * TaaS Load Test - k6
 * Run: k6 run test/load/load_test.js -e GATEWAY_URL=http://localhost:8080 -e API_KEY=your_key
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

const errorRate = new Rate('errors');
const inferenceLatency = new Trend('inference_latency', true);
const tokensThroughput = new Counter('tokens_total');

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';
const API_KEY = __ENV.API_KEY || '';
const MODEL = __ENV.MODEL || 'llama-3-8b';

export const options = {
  scenarios: {
    // Ramp up to steady state
    ramp_up: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 10 },
        { duration: '2m', target: 50 },
        { duration: '30s', target: 100 },
        { duration: '3m', target: 100 },
        { duration: '1m', target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],          // < 1% errors
    http_req_duration: ['p(95)<5000'],        // 95th percentile < 5s
    inference_latency: ['p(99)<10000'],       // 99th < 10s
  },
};

const headers = {
  'Content-Type': 'application/json',
  'X-API-Key': API_KEY,
};

const prompts = [
  'What is machine learning?',
  'Explain neural networks in simple terms.',
  'What are the benefits of cloud computing?',
  'How does tokenization work in NLP?',
  'Describe the transformer architecture.',
];

export default function () {
  const prompt = prompts[Math.floor(Math.random() * prompts.length)];

  const payload = JSON.stringify({
    model: MODEL,
    messages: [{ role: 'user', content: prompt }],
    max_tokens: 100,
    stream: false,
  });

  const start = Date.now();
  const res = http.post(`${GATEWAY_URL}/v1/chat/completions`, payload, {
    headers,
    timeout: '30s',
  });
  const latencyMs = Date.now() - start;

  const ok = check(res, {
    'status is 200': (r) => r.status === 200,
    'has choices': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.choices && body.choices.length > 0;
      } catch {
        return false;
      }
    },
  });

  errorRate.add(!ok);
  inferenceLatency.add(latencyMs);

  if (res.status === 200) {
    try {
      const body = JSON.parse(res.body);
      if (body.usage) {
        tokensThroughput.add(body.usage.total_tokens || 0);
      }
    } catch {}
  }

  sleep(Math.random() * 2 + 0.5);
}

export function handleSummary(data) {
  return {
    'test/load/results/summary.json': JSON.stringify(data, null, 2),
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
  };
}

function textSummary(data, opts) {
  return `
TaaS Load Test Summary
======================
Requests: ${data.metrics.http_reqs.values.count}
Failed:   ${(data.metrics.http_req_failed.values.rate * 100).toFixed(2)}%
p95 latency: ${data.metrics.http_req_duration.values['p(95)'].toFixed(0)}ms
p99 latency: ${data.metrics.http_req_duration.values['p(99)'].toFixed(0)}ms
Tokens processed: ${data.metrics.tokens_total?.values.count || 'N/A'}
`;
}
