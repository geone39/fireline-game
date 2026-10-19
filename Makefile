# Корневой Makefile (владелец: ops). Команды целей = документированные команды проверки модулей.
# Windows без make: docs/runbooks/dev-checks.md
.PHONY: check content econ sim server client wasm e2e e2e-down
check: content econ sim server client
content: ; python3 tools/validate_content.py
econ:    ; python3 tools/econ_sim.py --check > /dev/null
sim:     ; cd sim && go test ./...
# REQUIRE_DOCKER=1 (в CI) — тесты с testcontainers падают, если Docker недоступен
server:
	cd server && go vet ./...
	cd server && test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	cd server && go test ./...
client:
	cd client && npm ci && npm run lint && npm run typecheck && npm test && npm run build
# WASM-сборка ядра + паритет G01..G20 native/WASM через node (OPS-5). Не входит в check: паритет уже гоняет `sim`,
# если в PATH есть node; эта цель падает, а не пропускает тест, когда node нет.
wasm:
	command -v node > /dev/null || { echo "node not found: parity test would be skipped"; exit 1; }
	sh sim/wasm/build.sh
	@echo "sim.wasm: $$(wc -c < sim/wasm/dist/sim.wasm) bytes, gzip: $$(gzip -9c sim/wasm/dist/sim.wasm | wc -c) bytes"
	cd sim && go test -count=1 -v -run TestWasmParity ./wasm/wire/ | tee /tmp/wasm-parity.log
	@! grep -q -- '--- SKIP' /tmp/wasm-parity.log || { echo "TestWasmParity was skipped"; exit 1; }
# Сквозные тесты Playwright против docker compose (postgres+server+client). Не входит в check. Остановка: make e2e-down
E2E_COMPOSE = docker compose -f deploy/docker-compose.yml
e2e:
	$(E2E_COMPOSE) up -d --build --wait
	cd tests/e2e && npm ci && npx playwright install chromium && npx playwright test
e2e-down:
	$(E2E_COMPOSE) down -v
