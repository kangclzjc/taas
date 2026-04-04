import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const rateLimited = new Counter('rate_limited_requests');

export const options = {
  scenarios: {
    burst: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 200,
      maxDuration: '30s',
    },
  },
};

const BASE_URL = __ENV.TAAS_URL || 'http://localhost:8080';
const API_KEY = __ENV.TAAS_API_KEY || 'test-key';

export default function () {
  const res = http.post(`${BASE_URL}/v1/chat/completions`, JSON.stringify({
    model: 'llama-3-8b',
    messages: [{ role: 'user', content: 'ping' }],
    max_tokens: 1,
  }), {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_KEY}`,
    },
  });

  if (res.status === 429) {
    rateLimited.add(1);
    check(res, {
      'rate limit has retry-after or rate headers': (r) =>
        r.headers['X-Ratelimit-Limit'] !== undefined ||
        r.headers['Retry-After'] !== undefined,
    });
  }

  check(res, {
    'status is 200 or 429': (r) => r.status === 200 || r.status === 429,
  });
}
