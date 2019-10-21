# crio-status fish shell completion

function __fish_crio-status_no_subcommand --description 'Test if there has been any subcommand yet'
    for i in (commandline -opc)
        if contains -- $i config c info i containers container cs s complete completion help h
            return 1
        end
    end
    return 0
end

complete -c crio-status -n '__fish_crio-status_no_subcommand' -l socket -s s -r -d 'absolute path to the unix socket (default: "/var/run/crio/crio.sock")'
complete -c crio-status -n '__fish_crio-status_no_subcommand' -f -l help -s h -d 'show help'
complete -c crio-status -n '__fish_crio-status_no_subcommand' -f -l version -s v -d 'print the version'
complete -c crio-status -n '__fish_crio-status_no_subcommand' -f -l help -s h -d 'show help'
complete -c crio-status -n '__fish_crio-status_no_subcommand' -f -l version -s v -d 'print the version'
complete -c crio-status -n '__fish_seen_subcommand_from config c' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'config c' -d 'retrieve the configuration as a TOML string'
complete -c crio-status -n '__fish_seen_subcommand_from config c' -l socket -s s -r -d 'absolute path to the unix socket (default: "/var/run/crio/crio.sock")'
complete -c crio-status -n '__fish_seen_subcommand_from info i' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'info i' -d 'retrieve generic information'
complete -c crio-status -n '__fish_seen_subcommand_from info i' -l socket -s s -r -d 'absolute path to the unix socket (default: "/var/run/crio/crio.sock")'
complete -c crio-status -n '__fish_seen_subcommand_from containers container cs s' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'containers container cs s' -d 'retrieve information about containers'
complete -c crio-status -n '__fish_seen_subcommand_from containers container cs s' -l socket -s s -r -d 'absolute path to the unix socket (default: "/var/run/crio/crio.sock")'
complete -c crio-status -n '__fish_seen_subcommand_from containers container cs s' -f -l id -s i -r -d 'the container ID'
complete -c crio-status -n '__fish_seen_subcommand_from complete completion' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'complete completion' -d 'Output shell completion code'
complete -c crio-status -n '__fish_seen_subcommand_from help h' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'help h' -d 'Shows a list of commands or help for one command'
