param(
    [string]$StrataDir = 'C:\Users\gt50\Source\Repos\Strata',
    [int]$Port = 8080,
    [string]$DatabaseContainer = 'artex-postgres-1'
)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath($StrataDir)
$taskServer = Join-Path $taskRoot 'serve\server.py'
$taskConfigPath = Join-Path $taskRoot 'strata-unsloth-ud-iq4_xs.json'
$taskListener = @(Get-NetTCPConnection -State Listen -LocalPort $Port)
if ($taskListener.Count -ne 1 -or $taskListener[0].LocalAddress -ne '127.0.0.1') { throw 'Expected one loopback Strata listener' }
$taskServerPid = $taskListener[0].OwningProcess
$taskServerProcess = Get-CimInstance Win32_Process -Filter "ProcessId=$taskServerPid"
if ($taskServerProcess.CommandLine.IndexOf($taskServer, [StringComparison]::OrdinalIgnoreCase) -lt 0) { throw 'Listener is not the expected Strata server' }
$taskChildren = @(Get-CimInstance Win32_Process -Filter "ParentProcessId=$taskServerPid")
foreach ($taskChild in $taskChildren) {
    if (-not $taskChild.ExecutablePath.StartsWith($taskRoot + '\', [StringComparison]::OrdinalIgnoreCase)) { throw 'Unexpected Strata child process' }
}
$taskExisting = Get-Content -LiteralPath $taskConfigPath -Raw | ConvertFrom-Json
$taskHeaders = @{}
if ($taskExisting.api_key) { $taskHeaders.Authorization = 'Bearer ' + $taskExisting.api_key }
$taskMetrics = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/metrics" -Headers $taskHeaders -TimeoutSec 10
if ($taskMetrics.live.state -ne 'idle' -or [int]$taskMetrics.live.queued -ne 0) { throw 'Strata has active or queued work; retry when idle' }
$taskKey = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
$taskConfig = Get-Content -LiteralPath $taskConfigPath -Raw | ConvertFrom-Json
$taskConfig | Add-Member -NotePropertyName api_key -NotePropertyValue $taskKey -Force
$taskConfig | Add-Member -NotePropertyName host -NotePropertyValue '127.0.0.1' -Force
$taskIdentity = "$env:USERDOMAIN\$env:USERNAME"
icacls $taskConfigPath /inheritance:r /grant:r "${taskIdentity}:(F)" 'SYSTEM:(F)' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Unable to restrict Strata configuration access' }
[IO.File]::WriteAllText($taskConfigPath, ($taskConfig | ConvertTo-Json -Depth 30))
$taskSQL = "UPDATE llm_profiles SET api_key='$taskKey' WHERE id=1 AND name='Strata Local' AND base_url='http://host.docker.internal:8080/v1' RETURNING id;"
$taskResult = $taskSQL | docker exec -i $DatabaseContainer psql -v ON_ERROR_STOP=1 -U artex -d artex -At
if ($LASTEXITCODE -ne 0 -or $taskResult -notcontains '1') { throw 'Strata profile update failed; restore configuration from recovery archive before restarting' }
# Only stop the validated local listener and validated Strata engine children.
Stop-Process -Id $taskServerPid -Force
foreach ($taskChild in $taskChildren) { Stop-Process -Id $taskChild.ProcessId -Force -ErrorAction SilentlyContinue }
$taskLogDir = Join-Path $env:LOCALAPPDATA 'ARTEX\security-recovery\20261014-remediation'
New-Item -ItemType Directory -Path $taskLogDir -Force | Out-Null
$taskProcess = Start-Process -FilePath (Join-Path $taskRoot '.venv\Scripts\python.exe') -ArgumentList @(
    ('"' + $taskServer + '"'), '--engine', 'strata', '--config', ('"' + $taskConfigPath + '"'), '--port', "$Port"
) -WorkingDirectory $taskRoot -WindowStyle Hidden -RedirectStandardOutput (Join-Path $taskLogDir 'strata-start.stdout.log') -RedirectStandardError (Join-Path $taskLogDir 'strata-start.stderr.log') -PassThru
Write-Output "Started authenticated Strata launcher PID $($taskProcess.Id); key saved in protected configuration and ARTEX profile, not printed."
