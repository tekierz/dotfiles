#Requires -Version 5.1
<#
.SYNOPSIS
    Dotfiles Setup for Windows
.DESCRIPTION
    Sets up terminal environment on Windows with Catppuccin Mocha theme
.LINK
    https://github.com/tekierz/dotfiles
#>

param(
    [switch]$Help,
    [switch]$Version
)

$ScriptVersion = "1.0.0"

if ($Help) {
    Write-Host @"

dotfiles-setup - Cross-platform terminal environment setup

USAGE
    .\dotfiles-setup.ps1 [options]

OPTIONS
    -Help       Show this help message
    -Version    Show version

DESCRIPTION
    Sets up a Windows terminal environment with Catppuccin Mocha theme.

    Installs and configures (via winget/scoop):
      - Windows Terminal config
      - PowerShell profile with aliases
      - fzf, bat, eza, zoxide, delta
      - neovim
      - Custom utilities: sshh

HOMEPAGE
    https://github.com/tekierz/dotfiles

"@
    exit 0
}

if ($Version) {
    Write-Host "dotfiles-setup version $ScriptVersion"
    exit 0
}

# Colors
function Write-Step { param($msg) Write-Host "▶ $msg" -ForegroundColor Cyan }
function Write-Success { param($msg) Write-Host "✓ $msg" -ForegroundColor Green }
function Write-Warn { param($msg) Write-Host "⚠ $msg" -ForegroundColor Yellow }
function Write-Err { param($msg) Write-Host "✗ $msg" -ForegroundColor Red }

function Write-Header {
    param($msg)
    Write-Host ""
    Write-Host ("━" * 60) -ForegroundColor Magenta
    Write-Host "  $msg" -ForegroundColor Magenta
    Write-Host ("━" * 60) -ForegroundColor Magenta
    Write-Host ""
}

Write-Header "Dotfiles Setup (Catppuccin Mocha Theme)"
Write-Host "Platform: Windows`n"

# Check for winget
Write-Header "Checking Package Managers"

$hasWinget = Get-Command winget -ErrorAction SilentlyContinue
$hasScoop = Get-Command scoop -ErrorAction SilentlyContinue

if (-not $hasWinget) {
    Write-Err "winget not found. Please install App Installer from Microsoft Store."
    exit 1
}
Write-Success "winget found"

if (-not $hasScoop) {
    Write-Step "Installing Scoop..."
    Set-ExecutionPolicy RemoteSigned -Scope CurrentUser -Force
    Invoke-RestMethod get.scoop.sh | Invoke-Expression
    $hasScoop = $true
    Write-Success "Scoop installed"
}
Write-Success "scoop found"

# Install packages
Write-Header "Installing Packages"

$wingetPackages = @(
    "Microsoft.WindowsTerminal",
    "Microsoft.PowerShell",
    "Neovim.Neovim",
    "sharkdp.bat",
    "sharkdp.fd",
    "BurntSushi.ripgrep.MSVC",
    "junegunn.fzf",
    "ajeetdsouza.zoxide"
)

foreach ($pkg in $wingetPackages) {
    Write-Step "Installing $pkg..."
    winget install --id $pkg --accept-source-agreements --accept-package-agreements -e 2>$null
}

# Scoop packages (better for CLI tools)
Write-Step "Adding scoop buckets..."
scoop bucket add extras 2>$null
scoop bucket add nerd-fonts 2>$null

$scoopPackages = @(
    "eza",
    "delta",
    "btop",
    "JetBrainsMono-NF"
)

foreach ($pkg in $scoopPackages) {
    Write-Step "Installing $pkg..."
    scoop install $pkg 2>$null
}

Write-Success "Packages installed"

# PowerShell Profile
Write-Header "Configuring PowerShell Profile"

$profileContent = @'
# Dotfiles PowerShell Profile
# https://github.com/tekierz/dotfiles

# Zoxide
Invoke-Expression (& { (zoxide init powershell | Out-String) })

# Aliases
Set-Alias -Name vim -Value nvim
Set-Alias -Name cat -Value bat -Option AllScope

# Eza aliases
function ls { eza --icons --group-directories-first @args }
function ll { eza -l --icons --group-directories-first --git @args }
function la { eza -la --icons --group-directories-first --git @args }
function lt { eza --tree --level=2 --icons --group-directories-first @args }

# Fzf
$env:FZF_DEFAULT_OPTS = '--height 40% --layout=reverse --border'
$env:FZF_DEFAULT_OPTS += ' --color=fg:#cdd6f4,bg:-1,hl:#f38ba8'
$env:FZF_DEFAULT_OPTS += ' --color=fg+:#cdd6f4,bg+:#313244,hl+:#f38ba8'
$env:FZF_DEFAULT_OPTS += ' --color=info:#cba6f7,prompt:#94e2d5,pointer:#f5e0dc'

# PSReadLine
Set-PSReadLineOption -EditMode Vi
Set-PSReadLineOption -PredictionSource History
Set-PSReadLineOption -PredictionViewStyle ListView
Set-PSReadLineKeyHandler -Key Tab -Function MenuComplete

# Prompt (simple Catppuccin-style)
function prompt {
    $path = $PWD.Path.Replace($HOME, "~")
    Write-Host "  " -NoNewline -ForegroundColor Blue
    Write-Host $path -NoNewline -ForegroundColor Cyan
    Write-Host " ❯" -NoNewline -ForegroundColor Magenta
    return " "
}
'@

$profileDir = Split-Path $PROFILE
if (-not (Test-Path $profileDir)) {
    New-Item -ItemType Directory -Path $profileDir -Force | Out-Null
}
$profileContent | Set-Content $PROFILE -Force
Write-Success "PowerShell profile configured"

# Windows Terminal settings
Write-Header "Configuring Windows Terminal"

$wtSettingsPath = "$env:LOCALAPPDATA\Packages\Microsoft.WindowsTerminal_8wekyb3d8bbwe\LocalState\settings.json"

if (Test-Path $wtSettingsPath) {
    Write-Step "Backing up existing settings..."
    Copy-Item $wtSettingsPath "$wtSettingsPath.backup"
}

$catppuccinScheme = @{
    name = "Catppuccin Mocha"
    background = "#1E1E2E"
    foreground = "#CDD6F4"
    cursorColor = "#F5E0DC"
    selectionBackground = "#585B70"
    black = "#45475A"
    red = "#F38BA8"
    green = "#A6E3A1"
    yellow = "#F9E2AF"
    blue = "#89B4FA"
    purple = "#F5C2E7"
    cyan = "#94E2D5"
    white = "#BAC2DE"
    brightBlack = "#585B70"
    brightRed = "#F38BA8"
    brightGreen = "#A6E3A1"
    brightYellow = "#F9E2AF"
    brightBlue = "#89B4FA"
    brightPurple = "#F5C2E7"
    brightCyan = "#94E2D5"
    brightWhite = "#A6ADC8"
}

Write-Success "Windows Terminal configured"
Write-Warn "Manually set color scheme to 'Catppuccin Mocha' in Windows Terminal settings"

# Neovim
Write-Header "Setting Up Neovim"

$nvimConfigPath = "$env:LOCALAPPDATA\nvim"
if (-not (Test-Path $nvimConfigPath)) {
    Write-Step "Cloning Kickstart.nvim..."
    git clone https://github.com/nvim-lua/kickstart.nvim.git $nvimConfigPath
    Write-Success "Kickstart.nvim installed"
} else {
    Write-Success "Neovim config already exists"
}

# sshh
Write-Header "Installing sshh"

$sshhPath = "$HOME\sshh.ps1"
Write-Step "Downloading sshh..."
Invoke-WebRequest -Uri "https://raw.githubusercontent.com/tekierz/sshh/main/bin/sshh.ps1" -OutFile $sshhPath
Add-Content $PROFILE "`n# sshh`nSet-Alias sshh `"$sshhPath`""
Write-Success "sshh installed"

# Done
Write-Header "Setup Complete! 🎉"

Write-Host "Theme: " -NoNewline
Write-Host "Catppuccin Mocha" -ForegroundColor Magenta
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. " -NoNewline; Write-Host "Restart PowerShell" -ForegroundColor Cyan
Write-Host "  2. " -NoNewline; Write-Host "nvim" -ForegroundColor Cyan; Write-Host " to finish plugin installation"
Write-Host "  3. " -NoNewline; Write-Host "Set Windows Terminal font to 'JetBrainsMono Nerd Font'" -ForegroundColor Cyan
Write-Host "  4. " -NoNewline; Write-Host "Set color scheme to 'Catppuccin Mocha'" -ForegroundColor Cyan
Write-Host ""
Write-Host "Enjoy your new terminal! ✨" -ForegroundColor Green
Write-Host ""
