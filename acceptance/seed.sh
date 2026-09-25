#!/usr/bin/env bash
# Starts the stack, migrates, seeds one organization and prints its key as env lines.
set -euo pipefail
cd "$(dirname "$0")"
export SREAGENT_IMAGE_TAG="${SREAGENT_IMAGE_TAG:-$(cat IMAGE)}"
export SECRET_KEY_BASE="$(openssl rand -base64 64 | tr -d '\n')"
export CLOAK_KEY="$(openssl rand -base64 32)"
docker compose up -d db
docker compose run --rm app bin/sre_agent eval 'SreData.Release.migrate()'
docker compose up -d app
healthy=false
for _ in $(seq 1 60); do
  if curl -sf http://127.0.0.1:4000/api/health >/dev/null; then healthy=true; break; fi
  sleep 2
done
if [ "$healthy" != true ]; then
  echo "the app never answered /api/health; see: docker compose -f acceptance/docker-compose.yml logs app" >&2
  exit 1
fi
docker compose exec -T app bin/sre_agent eval 'SreData.Release.seed_tf_acceptance("tf-acc")' | grep -E '^SREAGENT_(API_KEY|READ_API_KEY|ACC_MEMBER_EMAIL|ORGANIZATION)='
echo "SREAGENT_BASE_URL=http://127.0.0.1:4000"
# The acceptance tests refuse to run without this line, so an environment
# holding a real key and nothing else can never reach a real organization.
echo "SREAGENT_ACC_STACK=local"
