# Корневой Makefile (владелец: ops). Команды целей = документированные команды проверки модулей.
# Windows без make: docs/runbooks/dev-checks.md
.PHONY: check content econ sim server client
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
