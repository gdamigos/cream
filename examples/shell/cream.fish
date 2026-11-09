# cream helpers for fish, copy this file to ~/.config/fish/conf.d/
#
#   cream_profile          prints the profile whose `cream rule` owns the current repo
#   CREAM_DIR_PROFILE      the same, updated before every prompt
#   cream_wrap claude gh   runs these tools as the repo's profile (through `cream run`)
#                          when it isn't the active one
#
# show the profile in your prompt, at the start of fish_prompt:
#   test -n "$CREAM_DIR_PROFILE"; and echo -n "[$CREAM_DIR_PROFILE] "
# or with starship:
#   [env_var.CREAM_DIR_PROFILE]
#   format = "[$env_value]($style) "

function cream_profile
    # cream includes <cream home>/git/<profile>.gitconfig for each rule, so the file git took
    # user.email from names the profile
    set -l origin (git config --show-origin --get user.email 2>/dev/null); or return 0
    string replace -rf '^.*/git/([^/]+)\.gitconfig"?\t.*$' '$1' -- $origin
end

function __cream_prompt --on-event fish_prompt
    set -gx CREAM_DIR_PROFILE (cream_profile)
end

function __cream_active
    set -l dir ~/.config/cream
    test -n "$XDG_CONFIG_HOME"; and set dir $XDG_CONFIG_HOME/cream
    test -n "$CREAM_HOME"; and set dir $CREAM_HOME
    test -f $dir/config.toml; and string replace -rf '^active = "(.*)"$' '$1' <$dir/config.toml
end

function __cream_exec
    set -l p (cream_profile)
    set -l active (__cream_active)
    # the active profile is already logged in and a `cream run` session has its own logins
    if test -z "$p"; or set -q CREAM_PROFILE; or test "$p" = "$active"
        command $argv
    else
        cream run $p $argv
    end
end

function cream_wrap
    for tool in $argv
        function $tool --inherit-variable tool
            __cream_exec $tool $argv
        end
    end
end
