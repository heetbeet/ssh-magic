param([switch]$Publish)
$ErrorActionPreference='Stop'
$go = if (Get-Command go -ErrorAction SilentlyContinue) { 'go' } else { Join-Path $PSScriptRoot '..\.tools\go\bin\go.exe' }
Push-Location (Join-Path $PSScriptRoot '..')
try {
 New-Item -ItemType Directory -Force dist | Out-Null
 $version=[regex]::Match((Get-Content main.go -Raw),'const version = "([^"]+)"').Groups[1].Value
 $env:CGO_ENABLED='0'; $env:GOARCH='amd64'
 $env:GOOS='windows'; & $go build -trimpath -ldflags='-s -w' -o dist/ssh-magic-windows-amd64.exe .; if($LASTEXITCODE){throw 'Windows build failed'}
 $env:GOOS='linux'; & $go build -trimpath -ldflags='-s -w' -o dist/ssh-magic-linux-amd64 .; if($LASTEXITCODE){throw 'Linux build failed'}
 $env:GOOS=$null
 $items=@(@('windows-amd64.exe','d50813aea4425c2113edcb889ffcc1a97f5a0d85517a35ef56114de45d8bda64','iroh-ssh.exe'),@('linux-amd64','6e39a6b22f14d683598600350ca642d3b67fd0b0a2f2a7b29928ad6cdf032826','iroh-ssh.linux'))
 foreach($i in $items) {
  $dst='dist/iroh-ssh-'+$i[0]
  if(-not(Test-Path $dst)){Invoke-WebRequest ('https://github.com/rustonbsd/iroh-ssh/releases/download/0.2.12/'+$i[2]) -OutFile $dst}
  if((Get-FileHash $dst).Hash -ne $i[1]){throw 'Iroh checksum mismatch'}
 }
 $winHash=(Get-FileHash dist/ssh-magic-windows-amd64.exe).Hash.ToLower()
 $linuxHash=(Get-FileHash dist/ssh-magic-linux-amd64).Hash.ToLower()
 & scripts/licenses.ps1 -Go $go
 $licenseHash=(Get-FileHash dist/licenses.zip).Hash.ToLower()
 $powershellBoot=(Get-Content scripts/bootstrap.ps1.in -Raw).Replace('@WINDOWS_MAGIC_SHA@',$winHash).Replace('@LICENSE_SHA@',$licenseHash)
 $bashBoot=(Get-Content scripts/bootstrap.sh.in -Raw).Replace('@LINUX_MAGIC_SHA@',$linuxHash).Replace('@LICENSE_SHA@',$licenseHash).Replace("`r`n","`n")
 foreach($path in @('boot.ps1','dist/bootstrap.ps1')) { [IO.File]::WriteAllText((Join-Path $PWD $path),$powershellBoot,[Text.UTF8Encoding]::new($false)) }
 foreach($path in @('boot.sh','dist/bootstrap.sh')) { [IO.File]::WriteAllText((Join-Path $PWD $path),$bashBoot,[Text.UTF8Encoding]::new($false)) }
 foreach($platform in @('windows','linux')) {
  $stage=Join-Path $PWD ('dist/offline-'+$platform)
  New-Item -ItemType Directory -Force $stage | Out-Null
  if($platform -eq 'windows') {
   Copy-Item dist/ssh-magic-windows-amd64.exe (Join-Path $stage 'ssh-magic.exe') -Force
   Copy-Item dist/iroh-ssh-windows-amd64.exe (Join-Path $stage 'iroh-ssh.exe') -Force
   Copy-Item scripts/offline-install.ps1 (Join-Path $stage 'install.ps1') -Force
  } else {
   Copy-Item dist/ssh-magic-linux-amd64 (Join-Path $stage 'ssh-magic') -Force
   Copy-Item dist/iroh-ssh-linux-amd64 (Join-Path $stage 'iroh-ssh') -Force
   Copy-Item scripts/offline-install.sh (Join-Path $stage 'install.sh') -Force
  }
  Copy-Item dist/licenses.zip,LICENSE,THIRD_PARTY.md $stage -Force
  [IO.File]::WriteAllText((Join-Path $stage 'README.txt'),"SSH Magic $version`nWindows: powershell -nop -ep bypass -file install.ps1`nLinux: bash install.sh`nThen reopen your terminal and run: ssh-magic open`nInstallation is offline. Connections require internet access.`nUninstall: ssh-magic remove`n",[Text.UTF8Encoding]::new($false))
  if($platform -eq 'windows') { Compress-Archive -Path "$stage/*" -DestinationPath dist/ssh-magic-windows-amd64.zip -Force }
  else { tar -czf dist/ssh-magic-linux-amd64.tar.gz -C $stage .; if($LASTEXITCODE){throw 'Linux bundle failed'} }
 }
 $assets=@('ssh-magic-windows-amd64.exe','ssh-magic-linux-amd64','iroh-ssh-windows-amd64.exe','iroh-ssh-linux-amd64','bootstrap.ps1','bootstrap.sh','licenses.zip','ssh-magic-windows-amd64.zip','ssh-magic-linux-amd64.tar.gz')
 $lines=foreach($name in $assets){(Get-FileHash ('dist/'+$name)).Hash.ToLower()+'  '+$name}
 [IO.File]::WriteAllText((Join-Path $PWD 'dist/SHA256SUMS'),($lines -join "`n")+"`n",[Text.UTF8Encoding]::new($false))
 if($Publish){ gh release create ('v'+$version) @($assets|ForEach-Object{'dist/'+$_}) dist/SHA256SUMS LICENSE THIRD_PARTY.md --prerelease --title ('SSH Magic '+$version) --notes-file RELEASE.md; if($LASTEXITCODE){throw 'Release failed'} }
} finally { $env:GOOS=$null; Pop-Location }
