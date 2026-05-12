# syntax=docker/dockerfile:1.7
# Build: docker build -t taas-dynamo-operator:local -f python/dynamo_operator/Dockerfile python/dynamo_operator
FROM python:3.11-slim-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 1001 taas && useradd -u 1001 -g taas -m -s /bin/bash taas

WORKDIR /app

RUN pip install --no-cache-dir uv

COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/uv \
    uv pip install --system -r requirements.txt

COPY . /app/dynamo_operator/

ENV PYTHONPATH=/app
USER taas

EXPOSE 8000

CMD ["python", "-m", "uvicorn", "dynamo_operator.main:app", "--host", "0.0.0.0", "--port", "8000"]
