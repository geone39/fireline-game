# One-shot builder: sim/wasm -> sim.wasm + wasm_exec.js in /out (compose bind-mounts client/public/sim there).
# Sources are bind-mounted (/src/sim), so every `compose up` rebuilds from the working tree; the Go build cache
# lives in a named volume, so unchanged code rebuilds in about a second. Same Go version as server.Dockerfile.
FROM golang:1.26-alpine
ENV GOOS=js GOARCH=wasm GOFLAGS=-trimpath CGO_ENABLED=0
WORKDIR /src/sim/wasm
ENTRYPOINT ["/bin/sh", "-c", "go build -ldflags='-s -w' -o /out/sim.wasm . && cp \"$(GOOS= GOARCH= go env GOROOT)/lib/wasm/wasm_exec.js\" /out/wasm_exec.js && ls -l /out"]
