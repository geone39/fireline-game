# Builds sim/wasm/dist/sim.wasm (+ wasm_exec.js from GOROOT).
$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    New-Item -ItemType Directory -Force dist | Out-Null
    $env:GOOS = 'js'; $env:GOARCH = 'wasm'
    go build -trimpath -ldflags='-s -w' -o dist/sim.wasm .
    Remove-Item Env:GOOS, Env:GOARCH
    Copy-Item (Join-Path (go env GOROOT) 'lib/wasm/wasm_exec.js') dist/wasm_exec.js
    (Get-Item dist/sim.wasm).Length
} finally { Pop-Location }
