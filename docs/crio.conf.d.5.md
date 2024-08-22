% crio.conf.d(5)

# NAME

crio.conf.d - directory for drop-in configuration files for CRI-O

# DESCRIPTION

Additionally to configuration in crio.conf(5), CRI-O allows to drop configuration
snippets into the crio.conf.d directory. The default directory is /etc/crio/crio.conf.d/.
The path can be changed via CRIO's **--config-dir** command line option.

# CONFIGURATION PRECEDENCE

When it exists, the main configuration file (/etc/crio/crio.conf by default) is
read before any file in the configuration directory (/etc/crio/crio.conf.d).
Settings in that file have the lowest precedence.

Files in the configuration directory are sorted by name in lexical order and
applied in that order. If multiple configuration files specify the same
configuration option the setting specified in file sorted last takes
precedence over any other value. That is if both 00-default.conf and
10-custom.conf exist in crio.conf.d and both specify different values for a
certain configuration option the value from 10-custom.conf will be applied.

# SEE ALSO

crio.conf(5), crio(8)
