#!/usr/bin/env bash
#
# Regenerate the Mixpanel Terraform provider from the frozen OpenAPI spec.
#
# Two modes:
#
#   ./gen/regen.sh            Default. Stage 4 only: re-run crudgen.py to emit
#                             internal/provider/*_resource.go, *_data_source.go,
#                             *_spec.go, examples, etc. from the COMMITTED IR
#                             (gen/provider_code_spec.json). This is the common
#                             case (new entity, CRUD/manifest tweak) and needs
#                             only Python + gofmt -- no external binaries.
#
#   ./gen/regen.sh --full     Stages 1-4. Rebuild the IR from the frozen spec
#                             too: preprocess -> tfplugingen-openapi ->
#                             postprocess -> tfplugingen-framework -> crudgen.
#                             Needed only when a schema attribute changes (i.e.
#                             after refreshing gen/spec/openapi.pruned.json).
#                             Requires tfplugingen-openapi + tfplugingen-framework
#                             on PATH (see gen/README.md).
#
# Run from the provider repo root.  Static (hand-written) files are never touched:
# internal/client/*, internal/provider/mock_test.go, main.go.
set -euo pipefail

GEN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$GEN_DIR")"
cd "$REPO_ROOT"

PY="${PYTHON:-python3}"
FULL=0
[[ "${1:-}" == "--full" ]] && FULL=1

if [[ "$FULL" == "1" ]]; then
  command -v tfplugingen-openapi   >/dev/null || { echo "ERROR: tfplugingen-openapi not on PATH (see gen/README.md)"; exit 1; }
  command -v tfplugingen-framework >/dev/null || { echo "ERROR: tfplugingen-framework not on PATH (see gen/README.md)"; exit 1; }

  echo "[1/4] preprocess spec -> gen/build/openapi.hashicorp.json"
  mkdir -p gen/build
  "$PY" gen/preprocess_spec.py

  echo "[2/4] tfplugingen-openapi -> gen/provider_code_spec.json"
  tfplugingen-openapi generate \
    --config gen/generator_config.yml \
    --output gen/provider_code_spec.json \
    gen/build/openapi.hashicorp.json

  echo "[2.5] postprocess IR (dedupe path/body name clashes, strip bad defaults)"
  "$PY" gen/postprocess_ir.py

  echo "[3/4] tfplugingen-framework -> internal/provider/{resource,datasource}_*"
  tfplugingen-framework generate all \
    --input gen/provider_code_spec.json \
    --output internal/provider
fi

echo "[4/4] crudgen -> internal/provider/*.go + examples"
"$PY" gen/crudgen.py

echo "done. Review with: git -C \"$REPO_ROOT\" status --short"
