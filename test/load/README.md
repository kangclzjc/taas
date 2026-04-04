# TaaS Load Tests

Load tests using [k6](https://k6.io/).

## Install k6

```bash
# macOS
brew install k6

# Linux
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D68
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update && sudo apt-get install k6
```

## Run Tests

```bash
# Start local environment
make docker-up

# Inference load test
k6 run test/load/k6_inference.js -e TAAS_URL=http://localhost:8080 -e TAAS_API_KEY=your-key

# Auth flow load test  
k6 run test/load/k6_auth.js -e TAAS_URL=http://localhost:8080

# Rate limit test
k6 run test/load/k6_rate_limit.js -e TAAS_URL=http://localhost:8080 -e TAAS_API_KEY=your-key
```

## SLA Targets

| Tier          | P99 Latency | Error Rate | Availability |
|---------------|-------------|------------|--------------|
| Standard      | < 5s        | < 1%       | 99.5%        |
| Professional  | < 2s        | < 0.5%     | 99.9%        |
| Enterprise    | < 1s        | < 0.1%     | 99.99%       |
