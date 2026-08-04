# Heimdall

**Idioma / Language:** [English](README.md) · [한국어](README.ko.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · **[Español](README.es.md)**

Heimdall es un evaluador **determinista y basado primero en evidencia** para harnesses de agentes locales de confianza. Congela un objetivo, ejecuta un comando declarado en una copia desechable, verifica artefactos explícitos, sella evidencia con contenido limitado y reduce los hard gates a uno de cuatro estados.

> **Idioma predeterminado de la documentación:** inglés. Este archivo es una traducción del README predeterminado en inglés.
>
> **Estado actual:** Go MVP con una release binaria inmutable y verificada mediante checksum. `v0.1.0` apunta exactamente al commit [`1cc04368`](https://github.com/JeremyDev87/heimdall/commit/1cc04368aebe25d459cc65796855a9f3e9ce3338) de `main`. Los gates de draft build, comparación de bytes remotos, runtime sin token y publicación inmutable pasaron en [workflow run 30908286471](https://github.com/JeremyDev87/heimdall/actions/runs/30908286471).

## Por qué Heimdall

Que un agente o harness diga `PASS`, `approved` o `read_only` no constituye evidencia independiente. Heimdall registra process exits, digests de artefactos, invariancia del código fuente, escrituras en los límites del workspace e identidad de la policy antes de producir un veredicto.

La ruta de autoridad es determinista. Este repositorio no contiene un LLM grader, puntuación numérica, auto-fixer, dashboard, approval service ni universal adapter.

## Release binaria inmutable

El canal estable de distribución es la [`release v0.1.0` inmutable](https://github.com/JeremyDev87/heimdall/releases/tag/v0.1.0). Contiene exactamente estos tres assets:

- `heimdall_0.1.0_linux_amd64.tar.gz`
- `heimdall_0.1.0_darwin_arm64.tar.gz`
- `checksums.txt`

Cada archive contiene únicamente `heimdall`, `LICENSE` y `README.md`. Como `v0.1.0` es una release inmutable etiquetada antes de esta actualización documental, el `README.md` incluido es la instantánea previa a la publicación del momento del tag y todavía indica que no existe una release binaria. La guía vigente de la release es el README más reciente de la default branch.

Verifica el checksum antes de extraer y después comprueba la provenance del runtime:

```bash
ARCHIVE=heimdall_0.1.0_darwin_arm64.tar.gz
curl -fLO "https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/${ARCHIVE}"
curl -fLO https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/checksums.txt
grep "  ${ARCHIVE}\$" checksums.txt | shasum -a 256 --check
tar -xzf "${ARCHIVE}"
./heimdall version
```

En Linux amd64 usa `ARCHIVE=heimdall_0.1.0_linux_amd64.tar.gz` en su lugar. La versión debe ser `0.1.0`, el commit debe ser `1cc04368aebe25d459cc65796855a9f3e9ce3338` y `build_date` debe estar presente.

El workflow demuestra la igualdad de bytes entre los artefactos del draft y los assets subidos por ese release run hosted. No se promete que una reconstrucción local con otro Go toolchain sea byte-identical; la autoridad de la release es el checksum publicado.

## Compilar desde el código fuente

Requisito: Go 1.26 o posterior. Heimdall no tiene una dependencia de runtime de Python ni de `uv`; un target command puede requerir de forma independiente el intérprete declarado en su manifest.

```bash
VERSION=dev
COMMIT="$(git rev-parse HEAD)"
if [ -n "$(git status --porcelain=v1 --untracked-files=normal --ignore-submodules=dirty)" ]; then
  COMMIT="${COMMIT}-dirty"
fi
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
go build -trimpath \
  -ldflags="-X github.com/JeremyDev87/heimdall/internal/cli.Version=${VERSION} -X github.com/JeremyDev87/heimdall/internal/cli.Commit=${COMMIT} -X github.com/JeremyDev87/heimdall/internal/cli.BuildDate=${BUILD_DATE}" \
  -o ./bin/heimdall ./cmd/heimdall
./bin/heimdall version
./bin/heimdall validate fixtures/pass/eval.yaml
./bin/heimdall evaluate fixtures/pass/eval.yaml --out /tmp/heimdall-pass
```

Para un nuevo command harness controlado por el propietario, inicializa un scaffold fail-closed:

```bash
/path/to/heimdall init --preset command-artifact --target . -- ./scripts/verify-harness.sh
/path/to/heimdall check
```

`init` nunca aprende la evidencia esperada de la primera ejecución ni sobrescribe un scaffold diferente. El command proporcionado debe verificar por sí mismo el resultado del harness; un command que solo termina con éxito no es un artifact oracle.

## Arquitectura MVP

```text
versioned eval manifest + policy
              │
              ▼
frozen target → trusted-local runner → deterministic checks
              │
              ▼
content-light evidence → hard-gate reducer → JSON + Markdown report
```

El manifest declara un target root relativo, una policy versionada, un command argv-only, un timeout y comprobaciones deterministas de archivos:

```yaml
schema_version: "1.0"
target:
  id: example-harness
  root: target
policy:
  id: harness-readiness
  version: "1"
  path: ../../policies/harness-readiness-v1.yaml
isolation: trusted-local
command:
  argv: [python3, run.py]
  timeout_seconds: 10
checks:
  - id: result
    kind: file_equals
    path: result.txt
    expected: "ok\n"
```

Los tipos de check admitidos son `file_exists`, `file_equals` y `path_absent`. Los campos desconocidos, claves YAML duplicadas, path traversal, policy drift y contratos mal formados fallan de forma cerrada.

## Estados y códigos de salida

| Estado | Exit | Significado |
| --- | ---: | --- |
| `PASS` | 0 | Todos los gates deterministas requeridos pasaron |
| `FAIL` | 1 | Falló un gate de verifier, command, no-write o límite del workspace |
| `BLOCKED` | 2 | No estaba disponible el contract, policy, target o prerrequisito de ejecución |
| `INCONCLUSIVE` | 3 | La ejecución fue incompleta o faltó evidencia requerida |

Los hard failures no se pueden compensar con promedios. En este MVP determinista, `failure_honesty` permanece en `N/A` en lugar de ser inventado por un modelo.

## Evidence contract

Una evaluación escribe exactamente tres artefactos:

- `evidence.json`: digests de target/policy, process receipt, content digests y check receipts;
- `report.json`: estado canónico, reason codes, criterios y semantic digest;
- `report.md`: proyección determinista de `report.json`.

No se copian en estos reports raw stdout, stderr, valores de entorno, contenido del target, credenciales ni rutas absolutas del target. Se representan únicamente mediante content digests y tamaños en bytes. Los documentos semánticos omiten timestamps y rutas temporales para que las ejecuciones repetidas sean comparables.

## Seguridad y límite de plataforma

`trusted-local` significa que el target es código controlado por el propietario. En runtimes hosted compatibles, Heimdall usa una copia temporal, subprocess argv-only, entorno reducido, limpieza de timeouts por grupo de procesos y verificación de no escritura en el código fuente.

**No es un sandbox del sistema operativo:** no impide el acceso a la red ni escrituras arbitrarias en el host. No evalúes código adversarial o de terceros con este runner. Esos targets requieren un backend futuro con container o sandbox impuesto por el host. La aceptación final y cada transición de estado externo siguen bajo control humano o del host.

El éxito de un cross-build es distinto de la evidencia del runtime hosted. El Hermes Skill incluido es distribuible, pero no se instala automáticamente.

## Real-target validation

El primer pilot trusted-local usó Heimdall `27c0aa105d7216bdc8d67ee3f544e3459422d7d0` contra `ddalggak` `89868c05ca781365701362db08666bca503901b2`. Dos evaluaciones positivas limpias produjeron el mismo evidence digest `d172d008de8152b7219c3f3b661219f1c3d265015936cc34ae5b069907cd1c98` y report digest `969e6a8e589bec08aee40ffdf0ee71b67677e37796026fb3092a702f8618c15b`, con evidencia target no-write verdadera. Un fixture-tamper probe terminó con exit `1` y se redujo a `FAIL` con `command_failed` y `required_evidence_missing`.

El hosted Linux real-target lane usa la revision exacta de Heimdall y la revision fijada de `ddalggak`, valida el manifest, evalúa dos veces y exige `PASS`, receipts correctos, semantic digests repetidos, target sin cambios y `outside_workspace_write=false`. Ese receipt acotado no observa escrituras arbitrarias del host ni convierte `trusted-local` en un sandbox del sistema. Comando de reproducción:

```bash
bash scripts/verify-ddalggak-real-target.sh \
  /path/to/ddalggak /tmp/heimdall-ddalggak-real-target \
  89868c05ca781365701362db08666bca503901b2 "$(git rev-parse HEAD)"
```

## Verificación y regression corpus

El corpus fijo cubre PASS con evidencia, self-report que parece PASS pero no tiene evidencia, receipts faltantes, escrituras fuera del target desechable y texto parecido a una inyección tratado como datos. Los tests de Go también cubren source mutation, non-zero exits, limpieza de descendientes tras timeout, cierre de schema, contención de output, claves duplicadas, rechazo de symlinks y redacción de credenciales.

```bash
test -z "$(gofmt -l .)"
go vet ./...
go test ./... -count=1
go test -race ./... -count=1
go build -trimpath -o /tmp/heimdall ./cmd/heimdall
```

## Repository layout

```text
cmd/heimdall/        CLI entrypoint
internal/            contract, runner, reducer, report y utilidades deterministas
schemas/             contratos strict v1 de input/evidence/report
policies/            artefactos de policy versionados
fixtures/            corpus fijo adversarial y benigno
testdata/oracle/     receipts congelados de paridad entre lenguajes
skill/heimdall/      Hermes Skill distribuible; no se instala en el profile
templates/           templates embedded del onboarding scaffold
```

## License y límite de distribución

El código fuente de Heimdall usa la [MIT License](LICENSE). La release binaria inmutable `v0.1.0` es un artefacto de distribución separado y verificado mediante checksum. No confundas la reutilización del código fuente, la redistribución binaria y las afirmaciones sobre un canal estable de instalación.

## Trabajo pendiente

Un reusable GitHub Action que consuma el contrato de release inmutable verificado queda como trabajo posterior. Container isolation, automatic approval, deployment y numeric scoring están fuera del alcance de este MVP.
