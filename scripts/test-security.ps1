param([switch]$Linux, [string]$LogPath = 'dist/security-go-tests-release.log')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
Set-Location $taskRoot
$taskValues = @{}
foreach ($taskLine in Get-Content -LiteralPath '.env') {
    if ($taskLine -match '^([A-Z_]+)=(.*)$') { $taskValues[$Matches[1]] = $Matches[2].Trim('"', "'") }
}
if (-not $taskValues.POSTGRES_PASSWORD) { throw 'POSTGRES_PASSWORD is required for the isolated local test database' }
$taskDatabase = 'artex_security_' + [guid]::NewGuid().ToString('N')
$taskUser = if ($taskValues.POSTGRES_USER) { $taskValues.POSTGRES_USER } else { 'artex' }
$taskPort = if ($taskValues.ARTEX_DB_PORT) { $taskValues.ARTEX_DB_PORT } else { '5434' }
$taskOldDSN = $env:ARTEX_PG_DSN
$taskOldAuthDSN = $env:ARTEX_AUTH_TEST_DSN
$taskExit = 1
$taskEnvPath = $null
try {
    docker exec artex-postgres-1 createdb -U $taskUser $taskDatabase
    if ($LASTEXITCODE -ne 0) { throw 'Unable to create isolated test database' }
    $env:ARTEX_PG_DSN = 'postgres://' + [uri]::EscapeDataString($taskUser) + ':' + [uri]::EscapeDataString($taskValues.POSTGRES_PASSWORD) + '@127.0.0.1:' + $taskPort + '/' + $taskDatabase + '?sslmode=disable'
    $env:ARTEX_AUTH_TEST_DSN = $env:ARTEX_PG_DSN
    if ($Linux) {
        docker build --target gobuild -t artex:security-test-build . *> dist/security-test-image-build.log
        if ($LASTEXITCODE -ne 0) { throw 'Linux test image build failed; see dist/security-test-image-build.log' }
        $taskContainerDSN = $env:ARTEX_PG_DSN -replace '@127\.0\.0\.1:\d+/', '@postgres:5432/'
        $taskEnvPath = Join-Path ([IO.Path]::GetTempPath()) ($taskDatabase + '.env')
        [IO.File]::WriteAllLines($taskEnvPath, @(('ARTEX_PG_DSN=' + $taskContainerDSN), ('ARTEX_AUTH_TEST_DSN=' + $taskContainerDSN)))
        docker run --rm --network artex_default --env-file $taskEnvPath artex:security-test-build go test -p 1 -count=1 ./... *> $LogPath
    } else {
        go test -p 1 -count=1 ./... *> $LogPath
    }
    $taskExit = $LASTEXITCODE
    Get-Content -LiteralPath $LogPath -Tail 20
} finally {
    $env:ARTEX_PG_DSN = $taskOldDSN
    $env:ARTEX_AUTH_TEST_DSN = $taskOldAuthDSN
    if ($taskEnvPath -and (Test-Path -LiteralPath $taskEnvPath)) { Remove-Item -LiteralPath $taskEnvPath }
    if ($taskDatabase -notmatch '^artex_security_[a-f0-9]{32}$') { throw 'Refusing unexpected database cleanup target' }
    docker exec artex-postgres-1 dropdb --if-exists --force -U $taskUser $taskDatabase
}
exit $taskExit
