package criocli

import (
	"fmt"
	"strings"

	"github.com/urfave/cli"
)

var Completion = cli.Command{
	Name:        "complete",
	Aliases:     []string{"completion"},
	Usage:       "Output shell completion code",
	ArgsUsage:   "SHELL",
	Description: "Output shell completion code for bash, zsh or fish.",
	Action: func(c *cli.Context) error {
		// select bash by default for backwards compatibility
		if c.NArg() == 0 {
			return bashCompletion(c)
		}

		if c.NArg() != 1 {
			return cli.ShowSubcommandHelp(c)
		}

		switch c.Args().First() {
		case "bash":
			return bashCompletion(c)
		case "fish":
			return fishCompletion(c)
		case "zsh":
			return zshCompletion(c)
		default:
			return fmt.Errorf("only bash, fish or zsh are supported")
		}
	},
}

const bashCompletionTemplate = `_cli_bash_autocomplete() {
    local cur opts base
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    opts="%s"
    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
    return 0
}

complete -F _cli_bash_autocomplete %s`

func bashCompletion(c *cli.Context) error {
	subcommands := []string{}
	for i := range c.App.Commands {
		command := &c.App.Commands[i]
		if command.Hidden {
			continue
		}
		for j := range command.Names() {
			subcommands = append(subcommands, command.Names()[j])
		}
	}

	for _, flag := range c.App.Flags {
		// only includes full flag name.
		subcommands = append(subcommands, "--"+strings.Split(flag.GetName(), ",")[0])
	}

	fmt.Fprintln(c.App.Writer,
		fmt.Sprintf(bashCompletionTemplate,
			strings.Join(subcommands, "\n"),
			c.App.Name))
	return nil
}

const zshCompletionTemplate = `_cli_zsh_autocomplete() {

  local -a cmds
  cmds=('%s')
  _describe 'commands' cmds

  local -a opts
  opts=('%s')
  _describe 'global options' opts

  return
}

compdef _cli_zsh_autocomplete %s`

func zshCompletion(c *cli.Context) error {
	subcommands := []string{}
	for i := range c.App.Commands {
		command := &c.App.Commands[i]
		if command.Hidden {
			continue
		}
		for _, name := range command.Names() {
			subcommands = append(subcommands, name+":"+command.Usage)
		}
	}

	opts := []string{}
	for _, flag := range c.App.Flags {
		// only includes full flag name.
		opts = append(opts, "--"+strings.Split(flag.GetName(), ",")[0])
	}

	fmt.Fprintln(c.App.Writer,
		fmt.Sprintf(zshCompletionTemplate,
			strings.Join(subcommands, "' '"),
			strings.Join(opts, "' '"),
			c.App.Name))
	return nil
}

func fishCompletion(c *cli.Context) error {
	completion, err := c.App.ToFishCompletion()
	if err != nil {
		return err
	}
	fmt.Fprintln(c.App.Writer, strings.TrimSpace(completion))
	return nil
}
