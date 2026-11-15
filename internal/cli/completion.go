package cli

import (
	"fmt"
	"strings"
)

// Shell completion scripts. Entry names are completed live from
// `sdbctl list --ids`, which is the same machine-readable surface the MCP
// server uses.

const verbsList = "status list boot default timeout set rm undo snapshots check edit mcp completion version help"

func completionScript(shell string) (string, error) {
	switch strings.ToLower(shell) {
	case "bash":
		return bashCompletion, nil
	case "zsh":
		return zshCompletion, nil
	case "fish":
		return fishCompletion, nil
	default:
		return "", fmt.Errorf("unsupported shell %q (try bash, zsh or fish)", shell)
	}
}

const bashCompletion = `# bash completion for sdbctl — eval "$(sdbctl completion bash)"
_sdbctl() {
    local cur verbs
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    verbs="` + verbsList + `"

    if [[ "$cur" == -* ]]; then
        COMPREPLY=( $(compgen -W "--esp --json --dry-run --force --yes --quiet --help" -- "$cur") )
        return 0
    fi

    case "${COMP_WORDS[1]}" in
        boot|default|rm)
            COMPREPLY=( $(compgen -W "$(sdbctl list --ids 2>/dev/null)" -- "$cur") )
            return 0
            ;;
        set)
            COMPREPLY=( $(compgen -W "$(sdbctl list --ids 2>/dev/null)" -- "$cur") )
            return 0
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
            return 0
            ;;
    esac

    if [[ ${#COMP_WORDS[@]} -le 2 ]]; then
        COMPREPLY=( $(compgen -W "$verbs" -- "$cur") )
    fi
    return 0
}
complete -o default -F _sdbctl sdbctl
`

const zshCompletion = `#compdef sdbctl
# zsh completion for sdbctl — save as _sdbctl in a fpath directory
_sdbctl() {
    local -a verbs
    verbs=(
        'status:show boot status'
        'list:list boot entries'
        'boot:boot one entry on the next restart'
        'default:set the default entry'
        'timeout:set the menu timeout'
        'set:set a loader.conf option'
        'rm:delete a boot entry'
        'undo:restore the last snapshot'
        'snapshots:list snapshots'
        'check:diagnostics, exit 1 on problems'
        'edit:launch the full-screen editor'
        'mcp:serve MCP over stdio'
        'completion:print a completion script'
        'version:print the version'
        'help:show usage'
    )

    if (( CURRENT == 2 )); then
        _describe 'verb' verbs
        return
    fi

    case "${words[2]}" in
        boot|default|rm)
            local -a entries
            entries=(${(f)"$(sdbctl list --ids 2>/dev/null)"})
            _describe 'entry' entries
            ;;
        timeout)
            _values 'timeout' '0' '1' '3' '5' 'menu-force' 'menu-hidden' 'menu-disabled'
            ;;
        completion)
            _values 'shell' 'bash' 'zsh' 'fish'
            ;;
        *)
            _arguments \
                '--esp[boot directory]:path:_files' \
                '--json[machine-readable output]' \
                '--dry-run[report without changing]' \
                '--force[act on hidden entries]' \
                '--yes[skip confirmations]' \
                '--quiet[less prose]'
            ;;
    esac
}
_sdbctl "$@"
`

const fishCompletion = `# fish completion for sdbctl — save as completions/sdbctl.fish
set -l sdbctl_verbs status list boot default timeout set rm undo snapshots check edit mcp completion version help

complete -c sdbctl -f -n "__fish_use_subcommand" -a "$sdbctl_verbs"
complete -c sdbctl -l esp -d 'Boot directory to manage' -x -a "(__fish_complete_directories)"
complete -c sdbctl -l json -d 'Machine-readable output'
complete -c sdbctl -l dry-run -d 'Report what would change, change nothing'
complete -c sdbctl -l force -d 'Act on entries hidden from the boot menu'
complete -c sdbctl -l yes -d 'Skip confirmation prompts'
complete -c sdbctl -l quiet -d 'Suppress explanatory prose'

complete -c sdbctl -f -n "__fish_seen_subcommand_from boot default rm" -a "(sdbctl list --ids 2>/dev/null)"
complete -c sdbctl -f -n "__fish_seen_subcommand_from timeout" -a "0 1 3 5 menu-force menu-hidden menu-disabled"
complete -c sdbctl -f -n "__fish_seen_subcommand_from completion" -a "bash zsh fish"
`
