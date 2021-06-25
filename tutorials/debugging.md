# Debugging CRI-O

Below is a non-comprehensive document on some tips and tricks for troubleshooting/debugging/inspecting the behavior of CRI-O.

### Printing go routines
Often with a long-running process, it can be useful to know what that process is up to.
CRI-O has built-in functionality to print the go routine stacks to provide such information.
All one has to do is send SIGUSR1 to CRI-O, either with `kill` or `systemctl` (if running CRI-O as a systemd unit):
```bash
kill -USR1 $crio-pid
systemctl kill -s USR1 crio.service
```

CRI-O will catch the signal, and write the routine stacks to `/tmp/crio-goroutine-stacks-$timestamp.log`
