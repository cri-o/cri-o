set -l __fish_crio_status_all_commands config containers help info

complete -f -c crio-status -s s -l socket -d "absolute path to the unix socket (default: '/var/run/crio/crio.sock')"
complete -f -c crio-status -s h -l help -d "show help"

complete -f -c crio-status  -n "not __fish_seen_subcommand_from $__fish_crio_status_all_commands" \
    -s v -l version -d "print the version"

complete -f -c crio-status -n "not __fish_seen_subcommand_from $__fish_crio_status_all_commands" \
    -a config -d "retrieve the configuration as TOML string"

complete -f -c crio-status -n "not __fish_seen_subcommand_from $__fish_crio_status_all_commands" \
    -a info -d "retrieve generic information"

complete -f -c crio-status -n "not __fish_seen_subcommand_from $__fish_crio_status_all_commands" \
    -a containers -d "retrieve information about containers"

complete -f -c crio-status -n "not __fish_seen_subcommand_from $__fish_crio_status_all_commands" \
    -a help -d "shows a list of commands or help for one command"
