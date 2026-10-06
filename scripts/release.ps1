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
 $env:GOOS='darwin'; & $go build -trimpath -ldflags='-s -w' -o dist/ssh-magic-darwin-amd64 .; if($LASTEXITCODE){throw 'macOS Intel build failed'}
 $env:GOARCH='arm64'; & $go build -trimpath -ldflags='-s -w' -o dist/ssh-magic-darwin-arm64 .; if($LASTEXITCODE){throw 'macOS Apple Silicon build failed'}
 $env:GOOS=$null; $env:GOARCH=$null
 # Iroh SSH 0.2.12 has no arm64 macOS release; scripts/build-iroh-macos.sh publishes one from the pinned source.
 $upstream='https://github.com/rustonbsd/iroh-ssh/releases/download/0.2.12/'
 $irohArm64=[regex]::Match((Get-Content runtime.go -Raw),'const irohDarwinARM64 = "([0-9a-f]{64})"').Groups[1].Value
 if(-not $irohArm64){throw 'Pin irohDarwinARM64 in runtime.go first'}
 $items=@(@('windows-amd64.exe','d50813aea4425c2113edcb889ffcc1a97f5a0d85517a35ef56114de45d8bda64',($upstream+'iroh-ssh.exe')),@('linux-amd64','6e39a6b22f14d683598600350ca642d3b67fd0b0a2f2a7b29928ad6cdf032826',($upstream+'iroh-ssh.linux')),@('darwin-amd64','064094cd96d51d1dfedecec37d01138dc521c69739ddc19b66367a7e318bca4a',($upstream+'iroh-ssh.macos')),@('darwin-arm64',$irohArm64,'https://github.com/heetbeet/ssh-magic/releases/download/iroh-ssh-0.2.12/iroh-ssh-darwin-arm64'))
 foreach($i in $items) {
  $dst='dist/iroh-ssh-'+$i[0]
  if(-not(Test-Path $dst)){Invoke-WebRequest $i[2] -OutFile $dst}
  if((Get-FileHash $dst).Hash -ne $i[1]){throw 'Iroh checksum mismatch'}
 }
 $winHash=(Get-FileHash dist/ssh-magic-windows-amd64.exe).Hash.ToLower()
 $linuxHash=(Get-FileHash dist/ssh-magic-linux-amd64).Hash.ToLower()
 $macIntelHash=(Get-FileHash dist/ssh-magic-darwin-amd64).Hash.ToLower()
 $macArmHash=(Get-FileHash dist/ssh-magic-darwin-arm64).Hash.ToLower()
 & scripts/licenses.ps1 -Go $go
 $licenseHash=(Get-FileHash dist/licenses.zip).Hash.ToLower()
 $powershellBoot=(Get-Content scripts/bootstrap.ps1.in -Raw).Replace('@WINDOWS_MAGIC_SHA@',$winHash).Replace('@LICENSE_SHA@',$licenseHash)
 $bashBoot=(Get-Content scripts/bootstrap.sh.in -Raw).Replace('@LINUX_MAGIC_SHA@',$linuxHash).Replace('@DARWIN_AMD64_MAGIC_SHA@',$macIntelHash).Replace('@DARWIN_ARM64_MAGIC_SHA@',$macArmHash).Replace('@DARWIN_ARM64_IROH_SHA@',$irohArm64).Replace('@LICENSE_SHA@',$licenseHash).Replace("`r`n","`n")
 foreach($path in @('boot.ps1','dist/bootstrap.ps1')) { [IO.File]::WriteAllText((Join-Path $PWD $path),$powershellBoot,[Text.UTF8Encoding]::new($false)) }
 foreach($path in @('boot.sh','dist/bootstrap.sh')) { [IO.File]::WriteAllText((Join-Path $PWD $path),$bashBoot,[Text.UTF8Encoding]::new($false)) }
 foreach($platform in @('windows-amd64','linux-amd64','darwin-amd64','darwin-arm64')) {
  $stage=Join-Path $PWD ('dist/offline-'+$platform)
  New-Item -ItemType Directory -Force $stage | Out-Null
  if($platform -eq 'windows-amd64') {
   Copy-Item dist/ssh-magic-windows-amd64.exe (Join-Path $stage 'ssh-magic.exe') -Force
   Copy-Item dist/iroh-ssh-windows-amd64.exe (Join-Path $stage 'iroh-ssh.exe') -Force
   Copy-Item scripts/offline-install.ps1 (Join-Path $stage 'install.ps1') -Force
  } else {
   Copy-Item ('dist/ssh-magic-'+$platform) (Join-Path $stage 'ssh-magic') -Force
   Copy-Item ('dist/iroh-ssh-'+$platform) (Join-Path $stage 'iroh-ssh') -Force
   Copy-Item scripts/offline-install.sh (Join-Path $stage 'install.sh') -Force
  }
  Copy-Item dist/licenses.zip,LICENSE,THIRD_PARTY.md $stage -Force
  [IO.File]::WriteAllText((Join-Path $stage 'README.txt'),"SSH Magic $version`nWindows: powershell -nop -ep bypass -file install.ps1`nLinux and macOS: bash install.sh`nThen reopen your terminal and run: ssh-magic open`nInstallation is offline. Connections require internet access.`nUninstall: ssh-magic remove`n",[Text.UTF8Encoding]::new($false))
  if($platform -eq 'windows-amd64') { Compress-Archive -Path "$stage/*" -DestinationPath dist/ssh-magic-windows-amd64.zip -Force }
  else { tar -czf ('dist/ssh-magic-'+$platform+'.tar.gz') -C $stage .; if($LASTEXITCODE){throw ($platform+' bundle failed')} }
 }
 $assets=@('ssh-magic-windows-amd64.exe','ssh-magic-linux-amd64','ssh-magic-darwin-amd64','ssh-magic-darwin-arm64','iroh-ssh-windows-amd64.exe','iroh-ssh-linux-amd64','iroh-ssh-darwin-amd64','iroh-ssh-darwin-arm64','bootstrap.ps1','bootstrap.sh','licenses.zip','ssh-magic-windows-amd64.zip','ssh-magic-linux-amd64.tar.gz','ssh-magic-darwin-amd64.tar.gz','ssh-magic-darwin-arm64.tar.gz')
 $lines=foreach($name in $assets){(Get-FileHash ('dist/'+$name)).Hash.ToLower()+'  '+$name}
 [IO.File]::WriteAllText((Join-Path $PWD 'dist/SHA256SUMS'),($lines -join "`n")+"`n",[Text.UTF8Encoding]::new($false))
 if($Publish){ gh release create ('v'+$version) @($assets|ForEach-Object{'dist/'+$_}) dist/SHA256SUMS LICENSE THIRD_PARTY.md --prerelease --title ('SSH Magic '+$version) --notes-file RELEASE.md; if($LASTEXITCODE){throw 'Release failed'} }
} finally { $env:GOOS=$null; $env:GOARCH=$null; Pop-Location }
