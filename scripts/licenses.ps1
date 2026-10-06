param([string]$Go='go')
$ErrorActionPreference='Stop'
$out=Join-Path $PWD 'dist/licenses'
New-Item -ItemType Directory -Force $out | Out-Null
function Collect($name,$dir) {
 $files=Get-ChildItem -LiteralPath $dir -File | Where-Object Name -match '^(LICENSE|COPYING|NOTICE|COPYRIGHT)'
 if($files){$dest=Join-Path $out ($name.Replace('/','_'));New-Item -ItemType Directory -Force $dest | Out-Null;$files | Copy-Item -Destination $dest -Force}
}
foreach($m in (& $Go list -m -f '{{.Path}}|{{.Dir}}' all)){$parts=$m.Split('|');if($parts.Length -eq 2 -and $parts[1]){Collect $parts[0] $parts[1]}}
Collect 'go-runtime' (& $Go env GOROOT)
$src=Join-Path $PWD 'docs/temp/iroh-source'
if(-not(Test-Path $src)){git clone --depth 1 --branch 0.2.12 https://github.com/rustonbsd/iroh-ssh.git $src;if($LASTEXITCODE){throw 'Iroh source download failed'}}
Collect 'iroh-ssh' $src
$metadata=cargo metadata --locked --format-version 1 --manifest-path (Join-Path $src 'Cargo.toml') | ConvertFrom-Json
if($LASTEXITCODE){throw 'Cargo dependency license collection failed'}
foreach($p in $metadata.packages){Collect ($p.name+'-'+$p.version) (Split-Path $p.manifest_path)}
$metadata.packages | Select-Object name,version,license,repository | ConvertTo-Json | Set-Content (Join-Path $out 'rust-packages.json') -Encoding UTF8
Copy-Item THIRD_PARTY.md $out -Force
Get-ChildItem $out -Recurse -File | ForEach-Object { $_.LastWriteTime=[datetime]'2026-01-01' }
Compress-Archive -Path "$out/*" -DestinationPath dist/licenses.zip -Force
