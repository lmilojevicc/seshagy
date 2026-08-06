package completion

import "fmt"

type Shell string

const (
	Bash Shell = "bash"
	Zsh  Shell = "zsh"
	Fish Shell = "fish"
)

func Script(shell Shell) (string, error) {
	switch shell {
	case Bash:
		return bashScript, nil
	case Zsh:
		return zshScript, nil
	case Fish:
		return fishScript, nil
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

const bashScript = `# bash completion for seshagy (Bash 3.2+)
_seshagy_complete() {
    local i line rest value description directive header current after prefix
    local -a argv candidates
    COMPREPLY=()
    argv=()
    for ((i=0; i<=COMP_CWORD; i++)); do
        argv[${#argv[@]}]="${COMP_WORDS[$i]}"
    done
    current="${COMP_WORDS[$COMP_CWORD]}"
    after="${COMP_LINE:COMP_POINT}"
    after="${after%%[[:space:]]*}"
    if [[ -n "$after" && "$current" == *"$after" ]]; then
        argv[${#argv[@]}-1]="${current:0:${#current}-${#after}}"
    fi
    prefix="${argv[${#argv[@]}-1]}"
    header=0
    candidates=()
    while IFS= read -r line; do
        if ((header == 0)); then
            case "$line" in
                $'v1\tnofiles') directive=nofiles ;;
                $'v1\tfiles') directive=files ;;
                $'v1\tdirs') directive=dirs ;;
                *) return 0 ;;
            esac
            header=1
            continue
        fi
        [[ "$line" == $'c\t'* ]] || return 0
        rest="${line#*$'\t'}"
        [[ "$rest" == *$'\t'* ]] || return 0
        value="${rest%%$'\t'*}"
        description="${rest#*$'\t'}"
        [[ -n "$value" && "$description" != *$'\t'* ]] || return 0
        candidates[${#candidates[@]}]=$value
    done < <(command seshagy __complete -- "${argv[@]}" 2>/dev/null)
    ((header == 1)) || return 0
    for value in "${candidates[@]}"; do
        COMPREPLY[${#COMPREPLY[@]}]=$value
    done
    if [[ "$directive" == "dirs" ]]; then
        while IFS= read -r value; do
            [[ "$value" == */ ]] || value="$value/"
            COMPREPLY[${#COMPREPLY[@]}]=$value
        done < <(compgen -d -- "$prefix")
    elif [[ "$directive" == "files" ]]; then
        while IFS= read -r value; do
            [[ ! -d "$value" || "$value" == */ ]] || value="$value/"
            COMPREPLY[${#COMPREPLY[@]}]=$value
        done < <(compgen -f -- "$prefix")
    fi
}
complete -F _seshagy_complete seshagy
`

const zshScript = `#compdef seshagy
_seshagy_complete() {
  local -a argv lines values descriptions
  local directive line rest value description
  argv=("${words[@][1,$((CURRENT - 1))]}" "$PREFIX")
  lines=("${(@f)$(command seshagy __complete -- "${argv[@]}" 2>/dev/null)}")
  (( ${#lines} > 0 )) || return 0
  case "${lines[1]}" in
    $'v1\tnofiles') directive=nofiles ;;
    $'v1\tfiles') directive=files ;;
    $'v1\tdirs') directive=dirs ;;
    *) return 0 ;;
  esac
  for line in "${lines[@]:1}"; do
    [[ "$line" == $'c\t'* ]] || return 0
    rest="${line#*$'\t'}"
    [[ "$rest" == *$'\t'* ]] || return 0
    value="${rest%%$'\t'*}"
    description="${rest#*$'\t'}"
    [[ -n "$value" && "$description" != *$'\t'* ]] || return 0
    values+=("$value")
    descriptions+=("$description")
  done
  (( ${#values} == 0 )) || compadd -d descriptions -- "${values[@]}"
  if [[ "$directive" == dirs ]]; then
    _files -/
  elif [[ "$directive" == files ]]; then
    _files
  fi
}
compdef _seshagy_complete seshagy
`

//nolint:dupword // Fish's "end" keyword necessarily appears for each nested block.
const fishScript = `function __seshagy_complete
    set -l current (commandline -ct)
    set -l lines (command seshagy __complete -- (commandline -opc) "$current" 2>/dev/null)
    test (count $lines) -gt 0; or return
    string match -rq '^v1\t(nofiles|files|dirs)$' -- $lines[1]; or return
    set -l header (string split \t -- $lines[1])
    set -l directive $header[2]
    set -l values
    set -l descriptions
    for line in $lines[2..-1]
        string match -rq '^c\t[^\t]+\t[^\t]*$' -- $line; or return
        set -l fields (string split \t -- $line)
        set -a values $fields[2]
        set -a descriptions $fields[3]
    end
    for i in (seq (count $values))
        printf '%s\t%s\n' (string escape -- $values[$i]) $descriptions[$i]
    end
    if test "$directive" = dirs
        __fish_complete_directories "$current"
    else if test "$directive" = files
        __fish_complete_path "$current"
    end
end
complete -c seshagy -f -a '(__seshagy_complete)'
`
