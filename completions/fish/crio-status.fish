# crio-status fish shell completion

function __fish_crio-status_no_subcommand --description 'Test if there has been any subcommand yet'
    for i in (commandline -opc)
        if contains -- $i complete completion help h man markdown md config c containers container cs s info i help h
            return 1
        end
    end
    return 0
end

complete -c crio-status -n '__fish_crio-status_no_subcommand' -l socket -s s -r -d 'absolute path to the unix socket'
complete -c crio-status -n '__fish_crio-status_no_subcommand' -f -l help -s h -d 'show help'
complete -c crio-status -n '__fish_crio-status_no_subcommand' -f -l version -s v -d 'print the version'
complete -c crio-status -n '__fish_crio-status_no_subcommand' -f -l help -s h -d 'show help'
complete -c crio-status -n '__fish_crio-status_no_subcommand' -f -l version -s v -d 'print the version'
complete -c crio-status -n '__fish_seen_subcommand_from complete completion' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'complete completion' -d 'Generate bash, fish or zsh completions.'
complete -c crio-status -n '__fish_seen_subcommand_from complete completion' -f -l help -s h -d 'show help'
complete -c crio-status -n '__fish_seen_subcommand_from help h' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_seen_subcommand_from complete completion' -a 'help h' -d 'Shows a list of commands or help for one command'
complete -c crio-status -n '__fish_seen_subcommand_from man' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'man' -d 'Generate the man page documentation.'
complete -c crio-status -n '__fish_seen_subcommand_from markdown md' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'markdown md' -d 'Generate the markdown documentation.'
complete -c crio-status -n '__fish_seen_subcommand_from config c' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'config c' -d 'Show the configuration of CRI-O as a TOML string.'
complete -c crio-status -n '__fish_seen_subcommand_from containers container cs s' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'containers container cs s' -d 'Display detailed information about the provided container ID.'
complete -c crio-status -n '__fish_seen_subcommand_from containers container cs s' -f -l id -s i -r -d 'the container ID'
complete -c crio-status -n '__fish_seen_subcommand_from info i' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'info i' -d 'Retrieve generic information about CRI-O, such as the cgroup and storage driver.'
complete -c crio-status -n '__fish_seen_subcommand_from help h' -f -l help -s h -d 'show help'
complete -r -c crio-status -n '__fish_crio-status_no_subcommand' -a 'help h' -d 'Shows a list of commands or help for one command'
