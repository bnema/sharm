#!/usr/bin/env sh
set -eu

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
compose_file="$repo_root/docker-compose.e2e.yml"
uid="$(id -u)"
if [ -n "$uid" ] && { [ -z "${DOCKER_HOST:-}" ] || [ "${DOCKER_HOST:-}" = "unix:///docker.sock" ]; }; then
  user_socket="/run/user/$uid/docker.sock"
  if [ -S "$user_socket" ]; then
    export DOCKER_HOST="unix://$user_socket"
  fi
fi

cleanup() {
  docker compose --project-directory "$repo_root" -f "$compose_file" down --volumes --remove-orphans
}
trap cleanup EXIT INT TERM

cleanup
docker compose --project-directory "$repo_root" -f "$compose_file" up --build --abort-on-container-exit --exit-code-from playwright playwright
logs="$(docker compose --project-directory "$repo_root" -f "$compose_file" logs --no-color sharm)"
printf '%s\n' "$logs" | grep -q 'video upload path=direct'
printf '%s\n' "$logs" | grep -q 'video upload path=server-fallback'
