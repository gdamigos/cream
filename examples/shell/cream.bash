# cream helpers for bash and zsh, source this file from ~/.bashrc or ~/.zshrc
#
#   cream_profile          prints the profile whose `cream rule` owns the current repo
#   CREAM_DIR_PROFILE      the same, updated before every prompt
#   cream_wrap claude gh   runs these tools as the repo's profile (through `cream run`)
#                          when it isn't the active one
#
# show the profile in your prompt:
#   bash      PS1='${CREAM_DIR_PROFILE:+[$CREAM_DIR_PROFILE] }'$PS1
#   zsh       setopt prompt_subst; PS1='${CREAM_DIR_PROFILE:+[$CREAM_DIR_PROFILE] }'$PS1
#   starship  [env_var.CREAM_DIR_PROFILE]
#             format = "[$env_value]($style) "

cream_profile() {
  # cream includes <cream home>/git/<profile>.gitconfig for each rule, so the file git took
  # user.email from names the profile
  local origin
  origin=$(git config --show-origin --get user.email 2>/dev/null) || return 0
  origin=${origin%%$'\t'*}
  origin=${origin%\"}
  case $origin in
    */git/*.gitconfig)
      origin=${origin##*/}
      printf '%s\n' "${origin%.gitconfig}"
      ;;
  esac
}

__cream_prompt() {
  local rc=$?
  CREAM_DIR_PROFILE=$(cream_profile)
  export CREAM_DIR_PROFILE
  return $rc
}

__cream_active() {
  local f=${CREAM_HOME:-${XDG_CONFIG_HOME:-$HOME/.config}/cream}/config.toml
  [ -f "$f" ] && sed -n 's/^active = "\(.*\)"$/\1/p' "$f"
}

__cream_exec() {
  local p
  p=$(cream_profile)
  # the active profile is already logged in and a `cream run` session has its own logins
  if [ -z "$p" ] || [ -n "${CREAM_PROFILE-}" ] || [ "$p" = "$(__cream_active)" ]; then
    command "$@"
  else
    cream run "$p" "$@"
  fi
}

cream_wrap() {
  local tool
  for tool in "$@"; do
    case $tool in
      *[!A-Za-z0-9_-]*) echo "cream_wrap: skipping '$tool'" >&2 ;;
      *) eval "$tool() { __cream_exec $tool \"\$@\"; }" ;;
    esac
  done
}

if [ -n "${ZSH_VERSION-}" ]; then
  autoload -Uz add-zsh-hook
  add-zsh-hook precmd __cream_prompt
else
  case ";${PROMPT_COMMAND-};" in
    *";__cream_prompt;"*) ;;
    *) PROMPT_COMMAND="__cream_prompt${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
  esac
fi
