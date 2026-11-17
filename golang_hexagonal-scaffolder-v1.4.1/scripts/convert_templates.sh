#!/usr/bin/env bash
# convert.sh — derive generator infrastructure templates from the
# generated-api reference source. Replaces the module path with the
# {{.ModulePath}} template action so the generator stamps the user's
# chosen module path into every emitted file.
#
# Non-test Go source in generated-api/ contains NO "{{" sequences
# (verified), so it is safe to pass through text/template with only the
# module-path action injected.
#
# Since 1.4.0.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)/.."
SRC="$ROOT/generated-api"
DST="$ROOT/generator/templates"

mkdir -p "$DST"

# Maps: source relative path → template name (the .tmpl file written).
# The generator's renderAll uses these names to route output paths.
declare -A MAP=(
  ["internal/shared/applogger/applogger.go"]="infra_applogger.go.tmpl"
  ["internal/shared/errors/errors.go"]="infra_errors.go.tmpl"
  ["internal/config/settings/settings.go"]="infra_settings.go.tmpl"
  ["internal/infra/database/databasefactory.go"]="infra_database.go.tmpl"
  ["internal/infra/bus/bus.go"]="infra_bus.go.tmpl"
  ["internal/app/middleware/middleware.go"]="infra_middleware.go.tmpl"
  ["internal/app/middleware/crypto.go"]="infra_crypto.go.tmpl"
  ["internal/app/routes/routes.go"]="infra_routes.go.tmpl"
  ["internal/app/routes/auth.go"]="infra_auth.go.tmpl"
  ["internal/app/server/server.go"]="infra_server.go.tmpl"
  ["internal/app/server/helpers.go"]="infra_helpers.go.tmpl"
)

for src_rel in "${!MAP[@]}"; do
  tmpl_name="${MAP[$src_rel]}"
  src="$SRC/$src_rel"
  out="$DST/$tmpl_name"
  {
    echo "{{/* template: $tmpl_name"
    echo "   Infrastructure template derived from the generated-api reference."
    echo "   {{.ModulePath}} is stamped into every import. Since 1.4.0. */}}"
    # Replace the module path; preserve everything else verbatim.
    sed 's|github.com/example/generated-api|{{.ModulePath}}|g' "$src"
  } > "$out"
  echo "wrote $tmpl_name"
done

echo "done"
