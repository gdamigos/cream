# cream helpers for PowerShell (5.1 and 7), dot-source this file from $PROFILE:
#   . C:\path\to\cream.ps1
#
#   Get-CreamProfile                  prints the profile whose `cream rule` owns the current repo
#   $env:CREAM_DIR_PROFILE            the same, updated before every prompt, which gets a [profile] prefix
#   Register-CreamWrapper claude gh   runs these tools as the repo's profile (through `cream run`)
#                                     when it isn't the active one
#
# with oh-my-posh or starship, dot-source this after their init and read CREAM_DIR_PROFILE there

function Get-CreamProfile {
    # cream includes <cream home>/git/<profile>.gitconfig for each rule, so the file git took
    # user.email from names the profile
    $origin = git config --show-origin --get user.email 2>$null
    if ($LASTEXITCODE -eq 0 -and "$origin" -match '[/\\]git[/\\]([^/\\]+)\.gitconfig"?\t') { $Matches[1] }
}

function Get-CreamActive {
    $dir = if ($env:CREAM_HOME) { $env:CREAM_HOME }
    elseif ($env:XDG_CONFIG_HOME) { Join-Path $env:XDG_CONFIG_HOME 'cream' }
    else { Join-Path $HOME '.config\cream' }
    $file = Join-Path $dir 'config.toml'
    if (Test-Path $file) {
        $m = Select-String -Path $file -Pattern '^active = "(.*)"$' | Select-Object -First 1
        if ($m) { $m.Matches[0].Groups[1].Value }
    }
}

function Invoke-CreamTool {
    $name, $exe = $args[0], $args[1]
    $rest = @($args | Select-Object -Skip 2)
    $p = Get-CreamProfile
    # the active profile is already logged in and a `cream run` session has its own logins
    if (-not $p -or $env:CREAM_PROFILE -or $p -eq (Get-CreamActive)) {
        & $exe @rest
    } else {
        cream run $p $name @rest
    }
}

function Register-CreamWrapper {
    foreach ($tool in $args) {
        $cmd = Get-Command $tool -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $cmd) { Write-Warning "Register-CreamWrapper: $tool not found"; continue }
        Set-Item "function:global:$tool" ([scriptblock]::Create("Invoke-CreamTool '$tool' '$($cmd.Source)' @args"))
    }
}

if (Get-Command git -ErrorAction SilentlyContinue) {
    $global:CreamOriginalPrompt = $function:prompt
    function global:prompt {
        $rc = $global:LASTEXITCODE
        $env:CREAM_DIR_PROFILE = Get-CreamProfile
        $global:LASTEXITCODE = $rc
        $prefix = if ($env:CREAM_DIR_PROFILE) { "[$env:CREAM_DIR_PROFILE] " } else { '' }
        $prefix + (& $global:CreamOriginalPrompt)
    }
}
