Боевое ядро. См. `modules/sim/AGENT.md`, `docs/design/battle-sim.md`. `go test ./...`. Сборка WASM: `GOOS=js GOARCH=wasm go build -o ../client/sim.wasm ./cmd/wasm` (cmd будет добавлен задачей SIM-4).

## WASM
`sim/wasm/` — сборка ядра для браузера (`build.ps1` / `build.sh` → `sim/wasm/dist/sim.wasm`), JSON-API `fireline_sim` и типы `api.d.ts`. Проверка паритета с native: `go test ./wasm/...` (нужны Go и Node).
