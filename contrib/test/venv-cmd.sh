#!/bin/bash

# This wrapper-script reduces the number of python-dependencies needed to execute a command
# and always executes from a fixed-version / verified environment. It only requires
# the following (or equivilent) be installed:
#
#    python2-virtualenv gcc openssl-devel redhat-rpm-config libffi-devel
#    python-devel python3-pycurl python-pycurl python2-simplejson util-linux
#
# Example usage (where ansible is NOT already installed)
#
#   $ ./venv-cmd ansible-playbook --version
#
# N/B: You may set $WORKSPACE and/or $ARTIFACTS to control where things are written

# All errors are fatal
set -e

echo

if [ "$#" -lt "1" ]
then
    echo "No command and command-line options specified."
    echo "usage: $0 <COMMAND> [OPTIONS...]"
    exit 3
fi

VENV_DIRNAME=".venv"
LOCKTIMEOUT_MINUTES="10"
SCRIPT_NAME=$(basename "$0")
SCRIPT_DIR=$(dirname `realpath "$0"`)
[ -n "$WORKSPACE" ] || export WORKSPACE="$SCRIPT_DIR"
export WORKSPACE=$(realpath $WORKSPACE)
mkdir -p "$WORKSPACE"
REQUIREMENTS="$WORKSPACE/requirements.txt"

# Confine this w/in the workspace
export PIPCACHE="$WORKSPACE/.cache/pip"
mkdir -p "$PIPCACHE"
# Don't recycle cache, it may become polluted between runs
trap 'rm -rf "$PIPCACHE" "$WORKSPACE/${VENV_DIRNAME}bootstrap"' EXIT

[ -n "$ARTIFACTS" ] || export ARTIFACTS="$WORKSPACE/artifacts"
export ARTIFACTS=$(realpath "$ARTIFACTS")
mkdir -p "$ARTIFACTS"
export LOGFILEPATH="$ARTIFACTS/$SCRIPT_NAME.log"

# All command failures from now on are fatal
set -e
echo "Bootstrapping trusted virtual environment, this may take a few minutes, depending on networking."
echo
echo "-----> Log: \"$LOGFILEPATH\")"
echo

(
  if ! flock --nonblock 42
  then
      echo "Another $SCRIPT_NAME virtual environment creation process is running."
      echo "Waiting up to $LOCKTIMEOUT_MINUTES minutes for it to exit."
      echo
      if ! flock --timeout $[60 * LOCKTIMEOUT_MINUTES] 42
      then
          echo "Could not obtain lock on virtual environment creation"
          echo
          exit 9
      fi
  fi
  echo "Virtual environment creation lock acquired"
  echo
  (
    set -x
    cd "$WORKSPACE"
    # When running more than once, make it fast by skipping the bootstrap
    if [ ! -d "./$VENV_DIRNAME" ] || [ ! -r "./$VENV_DIRNAME/.complete" ]; then
        # N/B: local system's virtualenv binary - uncontrolled version fixed below
        virtualenv --no-site-packages --python=python2 "./${VENV_DIRNAME}bootstrap"
        python3 -m venv --copies "./${VENV_DIRNAME}bootstrap"
        # Set up paths to install/operate out of $WORKSPACE/${VENV_DIRNAME}bootstrap
        source "./${VENV_DIRNAME}bootstrap/bin/activate"
        # N/B: local system's pip binary - uncontrolled version fixed below
        # pip may not support --cache-dir, force it's location into $WORKSPACE the ugly-way
        OLD_HOME="$HOME"
        export HOME="$WORKSPACE"
        pip install --force-reinstall --upgrade pip==9.0.1
        # Undo --cache-dir workaround
        export HOME="$OLD_HOME"
        # Install fixed, trusted, hashed versions of all requirements (including pip and virtualenv)
        pip --cache-dir="$PIPCACHE" install --force-reinstall --require-hashes \
            --requirement "$SCRIPT_DIR/requirements.txt"
        # Setup trusted virtualenv using hashed packages from requirements.txt
        "./${VENV_DIRNAME}bootstrap/bin/virtualenv" --no-site-packages --python=python2 "./$VENV_DIRNAME"
        "./${VENV_DIRNAME}bootstrap/bin/python3" -m venv --copies "./$VENV_DIRNAME"
        # Exit untrusted virtualenv
        deactivate
    fi
    # Enter trusted virtualenv
    source "./$VENV_DIRNAME/bin/activate"
    # Upgrade stock-pip to support hashes
    "./$VENV_DIRNAME/bin/pip" install --force-reinstall --cache-dir="$PIPCACHE" --upgrade pip==9.0.1
    # Re-install from cache but validate all hashes (including on pip itself)
    "./$VENV_DIRNAME/bin/pip" --cache-dir="$PIPCACHE" install --require-hashes \
        --requirement "$SCRIPT_DIR/requirements.txt"
    [ -r "./$VENV_DIRNAME/.complete" ] || echo "Setup by: $@" > "./$VENV_DIRNAME/.complete"
  ) &>> "$LOGFILEPATH"
) 42>>"$LOGFILEPATH"

# Since setup is complete, only kill the bootstrap on exit
trap 'rm -rf "$WORKSPACE/${VENV_DIRNAME}bootstrap"' EXIT

# Enter trusted virtualenv in this shell
source "$WORKSPACE/$VENV_DIRNAME/bin/activate"
echo "Executing $@"
echo
"$@"
deactivate  # just in case
