$ErrorActionPreference='Stop'
Push-Location (Join-Path $PSScriptRoot '../app')
try {
 go test ./...
 if ($LASTEXITCODE -ne 0) { throw 'Pruebas fallidas' }
 go run -buildvcs=false ./cmd/package @args
 if ($LASTEXITCODE -ne 0) { throw 'Compilación fallida' }
} finally { Pop-Location }
