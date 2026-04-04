import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  vus: 20,
  duration: '2m',
  thresholds: {
    http_req_duration: ['p(95)<1000'],
    errors: ['rate<0.01'],
  },
};

const BASE_URL = __ENV.TAAS_URL || 'http://localhost:8080';

export default function () {
  const uniqueEmail = `loadtest-${__VU}-${__ITER}-${Date.now()}@test.taas.io`;
  const headers = { 'Content-Type': 'application/json' };

  group('auth flow', () => {
    // Register
    const regRes = http.post(`${BASE_URL}/auth/register`, JSON.stringify({
      email: uniqueEmail,
      password: 'LoadTest1234!@#',
    }), { headers });

    const regOk = check(regRes, {
      'register 201': (r) => r.status === 201,
    });
    errorRate.add(!regOk);

    if (!regOk) return;

    const tokens = JSON.parse(regRes.body);

    // Login
    const loginRes = http.post(`${BASE_URL}/auth/login`, JSON.stringify({
      email: uniqueEmail,
      password: 'LoadTest1234!@#',
    }), { headers });

    check(loginRes, { 'login 200': (r) => r.status === 200 });

    // Access protected endpoint
    const meRes = http.get(`${BASE_URL}/tokens`, {
      headers: { ...headers, 'Authorization': `Bearer ${tokens.access_token}` },
    });
    check(meRes, { 'tokens 200': (r) => r.status === 200 });

    // Refresh
    const refreshRes = http.post(`${BASE_URL}/auth/refresh`, JSON.stringify({
      refresh_token: tokens.refresh_token,
    }), { headers });
    check(refreshRes, { 'refresh 200': (r) => r.status === 200 });
  });

  sleep(1);
}
