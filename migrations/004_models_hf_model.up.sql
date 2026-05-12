-- Hugging Face model id for NVIDIA DynamoGraphDeploymentRequest (and similar runtimes).
ALTER TABLE models ADD COLUMN IF NOT EXISTS hf_model TEXT NOT NULL DEFAULT '';
